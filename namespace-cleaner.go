package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Config struct {
	ClientID        string
	ClientSecret    string
	TenantID        string
	DryRun          bool
	AllowedDomains  []string
	GracePeriodDays int
}

func main() {
	cfg := Config{
		ClientID:        os.Getenv("CLIENT_ID"),
		ClientSecret:    os.Getenv("CLIENT_SECRET"),
		TenantID:        os.Getenv("TENANT_ID"),
		DryRun:          os.Getenv("DRY_RUN") == "true",
		AllowedDomains:  strings.Split(os.Getenv("ALLOWED_DOMAINS"), ","),
		GracePeriodDays: parseGracePeriod(os.Getenv("GRACE_PERIOD")),
	}

	ctx := context.Background()
	client := initGraphClient(ctx, cfg)
	kube := initKubeClient()

	processNamespaces(ctx, client, kube, cfg)
}

func parseGracePeriod(raw string) int {
	if days, err := fmt.Sscanf(raw, "%d", new(int)); err == nil {
		return days
	}
	return 30 // default fallback
}

func initGraphClient(ctx context.Context, cfg Config) *msgraphsdk.GraphServiceClient {
	cred, err := azidentity.NewClientSecretCredential(cfg.TenantID, cfg.ClientID, cfg.ClientSecret, nil)
	if err != nil {
		log.Fatalf("Failed to authenticate: %v", err)
	}
	authProvider, err := msgraphsdk.NewGraphRequestAdapter(ctx, cred, []string{"https://graph.microsoft.com/.default"})
	if err != nil {
		log.Fatalf("Failed to create Graph adapter: %v", err)
	}

	return msgraphsdk.NewGraphServiceClient(authProvider)
}

func initKubeClient() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to get in-cluster config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}
	return clientset
}

func userExists(ctx context.Context, client *msgraphsdk.GraphServiceClient, email string) bool {
	resp, err := client.Users().ByUserId(email).Get(ctx, nil)
	if err != nil {
		var httpErr *msgraphsdk.ODataError
		if ok := msgraphsdk.AsODataError(err, &httpErr); ok && httpErr.Response().StatusCode == http.StatusNotFound {
			return false
		}
		log.Printf("Error querying user %s: %v", email, err)
		return false
	}
	return resp != nil
}

func validDomain(email string, allowed []string) bool {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := parts[1]
	for _, d := range allowed {
		if d == domain {
			return true
		}
	}
	return false
}

func processNamespaces(ctx context.Context, graph *msgraphsdk.GraphServiceClient, kube *kubernetes.Clientset, cfg Config) {
	nsList, err := kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/part-of=kubeflow-profile,!namespace-cleaner/delete-at",
	})
	if err != nil {
		log.Fatalf("Failed to list namespaces: %v", err)
	}
	today := time.Now().UTC()
	graceDate := today.Add(time.Duration(cfg.GracePeriodDays) * 24 * time.Hour).Format("2006-01-02")

	for _, ns := range nsList.Items {
		email := ns.Annotations["owner"]
		if !validDomain(email, cfg.AllowedDomains) {
			log.Printf("Invalid domain for %s: %s", ns.Name, email)
			continue
		}
		if !userExists(ctx, graph, email) {
			log.Printf("User not found, marking namespace %s for deletion on %s", ns.Name, graceDate)
			if cfg.DryRun {
				log.Printf("[DryRun] Would label %s with delete-at=%s", ns.Name, graceDate)
			} else {
				patch := fmt.Sprintf(`{"metadata":{"labels":{"namespace-cleaner/delete-at":"%s"}}}`, graceDate)
				_, err := kube.CoreV1().Namespaces().Patch(ctx, ns.Name, metav1.TypeMergePatchType, []byte(patch), metav1.PatchOptions{})
				if err != nil {
					log.Printf("Failed to label namespace %s: %v", ns.Name, err)
				}
			}
		}
	}

	expired, err := kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "namespace-cleaner/delete-at",
	})
	if err != nil {
		log.Fatalf("Failed to list namespaces with delete-at: %v", err)
	}

	for _, ns := range expired.Items {
		email := ns.Annotations["owner"]
		label := ns.Labels["namespace-cleaner/delete-at"]
		parsedDate, _ := time.Parse("2006-01-02", label)
		if today.After(parsedDate) {
			if userExists(ctx, graph, email) {
				log.Printf("User %s restored, removing delete marker on %s", email, ns.Name)
				if cfg.DryRun {
					log.Printf("[DryRun] Would remove delete-at label from %s", ns.Name)
				} else {
					patch := `{"metadata":{"labels":{"namespace-cleaner/delete-at":null}}}`
					_, err := kube.CoreV1().Namespaces().Patch(ctx, ns.Name, metav1.TypeMergePatchType, []byte(patch), metav1.PatchOptions{})
					if err != nil {
						log.Printf("Failed to remove label: %v", err)
					}
				}
			} else {
				log.Printf("Deleting expired namespace: %s", ns.Name)
				if cfg.DryRun {
					log.Printf("[DryRun] Would delete namespace %s", ns.Name)
				} else {
					err := kube.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{})
					if err != nil {
						log.Printf("Failed to delete namespace: %v", err)
					}
				}
			}
		}
	}
}
