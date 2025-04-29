package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	msauth "github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	odataerrors "github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Config struct {
	ClientID       string
	ClientSecret   string
	TenantID       string
	DryRun         bool
	TestMode       bool
	AllowedDomains []string
	TestUsers      []string
	GracePeriod    int
}

const (
    labelTimeLayout = "2006-01-02_15-04-05Z"
)

func main() {
	cfg := Config{
		ClientID:       os.Getenv("CLIENT_ID"),
		ClientSecret:   os.Getenv("CLIENT_SECRET"),
		TenantID:       os.Getenv("TENANT_ID"),
		DryRun:         os.Getenv("DRY_RUN") == "true",
		TestMode:       os.Getenv("TEST_MODE") == "true",
		AllowedDomains: strings.Split(os.Getenv("ALLOWED_DOMAINS"), ","),
		TestUsers:      strings.Split(os.Getenv("TEST_USERS"), ","),
		GracePeriod:    getGracePeriod(),
	}

	ctx := context.Background()
	graphClient := initGraphClient(ctx, cfg)
	kubeClient := initKubeClient()

	processNamespaces(ctx, graphClient, kubeClient, cfg)
}

func getGracePeriod() int {
	val := os.Getenv("GRACE_PERIOD")
	if val == "" {
		return 30
	}
	var days int
	fmt.Sscanf(val, "%d", &days)
	return days
}

func initGraphClient(ctx context.Context, cfg Config) *msgraphsdk.GraphServiceClient {
	if cfg.TestMode {
		// Return mock client for tests
		return &msgraphsdk.GraphServiceClient{}
	}

	// Real authentication only in non-test mode
	cred, err := msauth.NewClientSecretCredential(
		cfg.TenantID,
		cfg.ClientID,
		cfg.ClientSecret,
		nil,
	)
	if err != nil {
		log.Fatalf("Graph auth failed: %v", err)
	}

	client, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, []string{"https://graph.microsoft.com/.default"})
	if err != nil {
		log.Fatalf("Graph client creation failed: %v", err)
	}
	return client
}

func initKubeClient() *kubernetes.Clientset {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Kubernetes config error: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Kubernetes client error: %v", err)
	}
	return clientset
}

func userExists(ctx context.Context, cfg Config, client *msgraphsdk.GraphServiceClient, email string) bool {
	if cfg.TestMode {
		for _, u := range cfg.TestUsers {
			if u == email {
				return true
			}
		}
		return false
	}

	_, err := client.Users().ByUserId(email).Get(ctx, nil)
	if err != nil {
		if respErr, ok := err.(*odataerrors.ODataError); ok {
			if mainError := respErr.GetErrorEscaped(); mainError != nil {
				if code := mainError.GetCode(); code != nil && *code == "NotFound" {
					return false
				}
			}
		}
		log.Printf("Error checking user %s: %v", email, err)
		return false
	}
	return true
}

func validDomain(email string, domains []string) bool {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := parts[1]
	for _, d := range domains {
		if d == domain {
			return true
		}
	}
	return false
}

func processNamespaces(ctx context.Context, graph *msgraphsdk.GraphServiceClient, kube kubernetes.Interface, cfg Config) {
	// Process namespaces without delete-at label
	nsList, err := kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/part-of=kubeflow-profile,!namespace-cleaner/delete-at",
	})
	if err != nil {
		log.Fatalf("Error listing namespaces: %v", err)
	}

	graceDate := time.Now().Add(time.Duration(cfg.GracePeriod) * 24 * time.Hour).UTC().Format(labelTimeLayout)

	for _, ns := range nsList.Items {
		email := ns.Annotations["owner"]
		if !validDomain(email, cfg.AllowedDomains) {
			log.Printf("Invalid domain: %s in ns %s", email, ns.Name)
			continue
		}
		if !userExists(ctx, cfg, graph, email) {
			log.Printf("Marking %s for deletion at %s", ns.Name, graceDate)
			if cfg.DryRun {
				log.Printf("[DRY RUN] Would label %s with delete-at=%s", ns.Name, graceDate)
			} else {
				patch := []byte(fmt.Sprintf(`{"metadata":{"labels":{"namespace-cleaner/delete-at":"%s"}}}`, graceDate))				_, err := kube.CoreV1().Namespaces().Patch(ctx, ns.Name, types.MergePatchType, patch, metav1.PatchOptions{})
				if err != nil {
					log.Printf("Error patching %s: %v", ns.Name, err)
				}
			}
		}
	}

	// Process namespaces with delete-at label
	expired, err := kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "namespace-cleaner/delete-at",
	})
	if err != nil {
		log.Fatalf("Error listing expired namespaces: %v", err)
	}
	today := time.Now()

	for _, ns := range expired.Items {
		labelValue := ns.Labels["namespace-cleaner/delete-at"]
		
		// Replace underscores to restore RFC3339 format
		labelValue = strings.Replace(labelValue, "_", "T", 1)
		labelValue = strings.Replace(labelValue, "_", ":", -1)
		
		deletionDate, err := time.Parse(time.RFC3339, labelValue)
		if err != nil {
			log.Printf("Failed to parse delete-at label %q: %v", labelValue, err)
			continue
		}

		email := ns.Annotations["owner"]

		// Check if user exists FIRST
		if userExists(ctx, cfg, graph, email) {
			log.Printf("User restored, removing delete-at from %s", ns.Name)
			if cfg.DryRun {
				log.Printf("[DRY RUN] Would remove label from %s", ns.Name)
			} else {
				patch := []byte(`{"metadata":{"labels":{"namespace-cleaner/delete-at":null}}}`)
				_, err := kube.CoreV1().Namespaces().Patch(ctx, ns.Name, types.MergePatchType, patch, metav1.PatchOptions{})
				if err != nil {
					log.Printf("Error removing label: %v", err)
				}
			}
		} else if today.After(deletionDate) {
			// Only delete if user doesn't exist AND date has passed
			log.Printf("Deleting namespace: %s", ns.Name)
			if cfg.DryRun {
				log.Printf("[DRY RUN] Would delete %s", ns.Name)
			} else {
				err := kube.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{})
				if err != nil {
					log.Printf("Error deleting ns: %v", err)
				}
			}
		}
	}
}
