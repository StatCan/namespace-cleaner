package main

import (
	"context"
	"errors"
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

// Config holds configuration for namespace-cleaner
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
	// labelTimeLayout is the format used for delete-at labels
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
	kubeClient, err := initKubeClient(nil)
	if err != nil {
		log.Fatalf("Failed to initialize Kubernetes client: %v", err)
	}

	processNamespaces(ctx, graphClient, kubeClient, cfg)
}

// getGracePeriod reads GRACE_PERIOD env or defaults to 30 days
func getGracePeriod() int {
	val := os.Getenv("GRACE_PERIOD")
	if val == "" {
		return 30
	}
	var days int
	_, err := fmt.Sscanf(val, "%d", &days)
	if err != nil || days < 0 {
		return 0
	}
	return days
}

// initGraphClient initializes Microsoft Graph client (or mock in test mode)
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

	client, err := msgraphsdk.NewGraphServiceClientWithCredentials(
		cred,
		[]string{"https://graph.microsoft.com/.default"},
	)
	if err != nil {
		log.Fatalf("Graph client creation failed: %v", err)
	}
	return client
}

// initKubeClient initializes Kubernetes in-cluster client or returns an error
func initKubeClient(cfg *rest.Config) (*kubernetes.Clientset, error) {
	if cfg != nil {
		return kubernetes.NewForConfig(cfg)
	}

	// Try in-cluster config (only available inside Kubernetes pods)
	if inClusterCfg, err := rest.InClusterConfig(); err == nil {
		return kubernetes.NewForConfig(inClusterCfg)
	}

	// Fall back to mockable out-of-cluster config if KUBECONFIG is set
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return kubernetes.NewForConfig(&rest.Config{
			Host: "http://localhost:8080",
		})
	}

	// Default failure case
	return nil, errors.New("no valid Kubernetes config found")
}

// userExists checks if a user exists in Graph (or test list)
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

// validDomain ensures email's domain is in allowed list
func validDomain(email string, domains []string) bool {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := parts[1]

	for _, allowed := range domains {
		if domain == allowed || strings.HasSuffix(domain, "."+allowed) {
			return true
		}
	}
	return false
}

// processNamespaces labels or deletes namespaces based on owner existence and delete-at
func processNamespaces(ctx context.Context, graph *msgraphsdk.GraphServiceClient, kube kubernetes.Interface, cfg Config) {
	// 1) Add delete-at label to namespaces missing it
	nsList, err := kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/part-of=kubeflow-profile,!namespace-cleaner/delete-at",
	})
	if err != nil {
		log.Fatalf("Error listing namespaces: %v", err)
	}

	// compute grace date
	graceDate := time.Now().Add(time.Duration(cfg.GracePeriod) * 24 * time.Hour).
		UTC().Format(labelTimeLayout)

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
				patch := []byte(fmt.Sprintf(`{"metadata":{"labels":{"namespace-cleaner/delete-at":"%s"}}}`, graceDate))
				_, err = kube.CoreV1().Namespaces().Patch(
					ctx,
					ns.Name,
					types.MergePatchType,
					patch,
					metav1.PatchOptions{},
				)
				if err != nil {
					log.Printf("Error patching %s: %v", ns.Name, err)
				}
			}
		}
	}

	// 2) Remove label or delete expired namespaces
	expired, err := kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "namespace-cleaner/delete-at",
	})
	if err != nil {
		log.Fatalf("Error listing expired namespaces: %v", err)
	}
	today := time.Now()

	for _, ns := range expired.Items {
		email := ns.Annotations["owner"]

		labelValue := ns.Labels["namespace-cleaner/delete-at"]
		// parse using custom layout directly
		deletionDate, err := time.Parse(labelTimeLayout, labelValue)
		if err != nil {
			log.Printf("Failed to parse delete-at label %q: %v", labelValue, err)
			// Remove invalid label
			if !cfg.DryRun {
				patch := []byte(`{"metadata":{"labels":{"namespace-cleaner/delete-at":null}}}`)
				_, err := kube.CoreV1().Namespaces().Patch(ctx, ns.Name, types.MergePatchType, patch, metav1.PatchOptions{})
				if err != nil {
					log.Printf("Error removing invalid label: %v", err)
				}
			}
			continue
		}

		// if user reappeared, remove label
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
			// if still missing and past date, delete namespace
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
