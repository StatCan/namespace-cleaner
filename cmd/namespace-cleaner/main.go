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
	/*
		client id -
		client secret -
		tenant id -
		dry run - simulates a run without actual modifications
		test mode - mocks certain values for testing
		allowed domains - allowed email domains for user validation
		test users - list of emails to simulate test runs with
		grace period - how many days until a namespace is marked stale
	*/

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

	graphClient := initGraphClient(cfg)
	kubeClient, err := initKubeClient(nil)
	if err != nil {
		log.Fatalf("Failed to initialize Kubernetes client: %v", err)
	}

	processNamespaces(context.Background(), graphClient, kubeClient, cfg)
}

// getGracePeriod reads GRACE_PERIOD env or defaults to 30 days
func getGracePeriod() int {
	val := os.Getenv("GRACE_PERIOD")
	if val == "" {
		log.Printf("GRACE_PERIOD empty. Defaulting to 30.")
		return 30
	}
	var days int
	_, err := fmt.Sscanf(val, "%d", &days) // val: str -> days: int conversion

	if err != nil {
		log.Printf("Bad argument for GRACE_PERIOD. Defaulting to 0.")
		return 0
	}
	if days < 0 {
		log.Printf("Days cannot be negative. Defaulting to 0.")
		return 0
	}

	return days
}

// initGraphClient initializes Microsoft Graph client (or mock in test mode)
func initGraphClient(cfg Config) *msgraphsdk.GraphServiceClient {
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
		[]string{"https://graph.microsoft.com/.default "},
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
		// Check for structured Microsoft Graph "NotFound" error
		if respErr, ok := err.(*odataerrors.ODataError); ok {
			if mainError := respErr.GetErrorEscaped(); mainError != nil {
				if code := mainError.GetCode(); code != nil && *code == "NotFound" {
					return false
				}
			}
		}

		// Fallback: look for "does not exist" in plain error text
		if strings.Contains(err.Error(), "does not exist") {
			return false
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
	graceDate := time.Now().Add(time.Duration(cfg.GracePeriod) * 24 * time.Hour).UTC().Format(labelTimeLayout)

	// Stats counters
	totalNamespaces := 0
	toLabel := 0
	toDelete := 0
	toRemoveLabel := 0
	invalidLabels := 0
	skippedMissingOwner := 0
	skippedInvalidDomain := 0
	skippedExistingUser := 0

	// Phase 1: Add delete-at label to namespaces without it
	log.Printf("[DEBUG] Phase 1: Looking for namespaces needing delete-at label...")
	nsList, err := kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/part-of=kubeflow-profile,!namespace-cleaner/delete-at",
	})
	if err != nil {
		log.Fatalf("Error listing namespaces: %v", err)
	}

	log.Printf("[DEBUG] Found %d matching namespaces", len(nsList.Items))
	totalNamespaces += len(nsList.Items)

	for _, ns := range nsList.Items {
		email, found := ns.Annotations["owner"]
		if !found {
			if cfg.DryRun {
				log.Printf("[DRY RUN] - Owner annotation: MISSING")
				log.Printf("[DRY RUN] SKIPPED: Missing 'owner' annotation")
			}
			skippedMissingOwner++
			continue
		}

		if cfg.DryRun {
			log.Printf("[DRY RUN] - Owner annotation: %s", email)
		}

		if cfg.DryRun {
			log.Printf("[DRY RUN] Examining namespace: %s", ns.Name)
			log.Printf("[DRY RUN] - LabelSelector match: YES")
		}

		if !validDomain(email, cfg.AllowedDomains) {
			if cfg.DryRun {
				log.Printf("[DRY RUN] - Domain check: NO")
				log.Printf("[DRY RUN] SKIPPED: Invalid domain for %s", email)
			}
			skippedInvalidDomain++
			continue
		}

		if cfg.DryRun {
			log.Printf("[DRY RUN] - Domain check: YES")
		}

		exists := userExists(ctx, cfg, graph, email)

		if cfg.DryRun {
			log.Printf("[DRY RUN] - User exists in AD: %v", exists)
		}

		if exists {
			if cfg.DryRun {
				log.Printf("[DRY RUN] SKIPPED: Owner still exists")
			}
			skippedExistingUser++
			continue
		}

		if cfg.DryRun {
			log.Printf("[DRY RUN] ACTION: Would label with delete-at=%s", graceDate)
		} else {
			log.Printf("Marking %s for deletion at %s", ns.Name, graceDate)

			patch := []byte(fmt.Sprintf(`{"metadata":{"labels":{"namespace-cleaner/delete-at":"%s"}}}`, graceDate))
			_, err = kube.CoreV1().Namespaces().Patch(ctx, ns.Name, types.MergePatchType, patch, metav1.PatchOptions{})
			if err != nil {
				log.Printf("Error patching %s: %v", ns.Name, err)
			}
		}

		toLabel++

	}

	// Phase 2: Delete expired namespaces or remove label if user reappeared
	log.Printf("[DEBUG] Phase 2: Checking for expired namespaces...")
	expired, err := kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "namespace-cleaner/delete-at",
	})
	if err != nil {
		log.Fatalf("Error listing expired namespaces: %v", err)
	}
	today := time.Now().UTC()

	log.Printf("[DEBUG] Found %d expired namespaces", len(expired.Items))
	totalNamespaces += len(expired.Items)

	for _, ns := range expired.Items {
		email, found := ns.Annotations["owner"]
		if !found {
			if cfg.DryRun {
				log.Printf("[DRY RUN] SKIPPED: Missing 'owner' annotation")
			}
			skippedMissingOwner++
			continue
		}

		labelValue := ns.Labels["namespace-cleaner/delete-at"]

		deletionDate, err := time.ParseInLocation(labelTimeLayout, labelValue, time.UTC)
		if err != nil {
			if cfg.DryRun {
				log.Printf("[DRY RUN] Namespace %s has invalid delete-at label: %q", ns.Name, labelValue)
			}
			invalidLabels++
			continue
		}

		if cfg.DryRun {
			log.Printf("[DRY RUN] Examining expired namespace: %s", ns.Name)
			log.Printf("[DRY RUN] - Owner annotation: %s", email)
			log.Printf("[DRY RUN] - Expiry date: %s", labelValue)
			log.Printf("[DRY RUN] - Today: %s", today.Format(labelTimeLayout))
		}

		if !validDomain(email, cfg.AllowedDomains) {
			if cfg.DryRun {
				log.Printf("[DRY RUN] SKIPPED: Invalid domain for %s", email)
			}
			skippedInvalidDomain++
			continue
		}

		exists := userExists(ctx, cfg, graph, email)

		if cfg.DryRun {
			log.Printf("[DRY RUN] - User exists in AD: %v", exists)
		}

		if exists {
			if cfg.DryRun {
				log.Printf("[DRY RUN] ACTION: Would remove delete-at label")
			} else {
				log.Printf("Removing delete-at label from %s", ns.Name)
				patch := []byte(`{"metadata":{"labels":{"namespace-cleaner/delete-at":null}}}`)
				_, err := kube.CoreV1().Namespaces().Patch(ctx, ns.Name, types.MergePatchType, patch, metav1.PatchOptions{})
				if err != nil {
					log.Printf("Error removing label: %v", err)
				}
			}
			toRemoveLabel++
		} else if today.After(deletionDate) {
			if cfg.DryRun {
				log.Printf("[DRY RUN] ACTION: Would delete namespace")
			} else {
				log.Printf("Deleting namespace %s", ns.Name)
				if cfg.TestMode {
					nsCopy := ns.DeepCopy()
					nsCopy.Finalizers = nil
					_, err := kube.CoreV1().Namespaces().Update(ctx, nsCopy, metav1.UpdateOptions{})
					if err != nil {
						log.Printf("Error removing finalizers from %s: %v", ns.Name, err)
					}
				}

				err := kube.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{})
				if err != nil {
					log.Printf("Error deleting ns: %v", err)
				} else {
					log.Printf("Deletion initiated for %s", ns.Name)
				}
			}
			toDelete++
		} else {
			if cfg.DryRun {
				log.Printf("[DRY RUN] STATUS: Not yet expired")
			} else {
				log.Printf("Namespace %s not yet expired", ns.Name)
			}
		}
	}

	// === [DRY RUN] Summary Table ===
	if cfg.DryRun {
		log.Printf("\n============================")
		log.Printf("[DRY RUN] Summary")
		log.Printf("----------------------------")
		log.Printf("Namespaces checked:         %d", totalNamespaces)
		log.Printf("Would label:                %d", toLabel)
		log.Printf("Would delete:               %d", toDelete)
		log.Printf("Would remove label:         %d", toRemoveLabel)
		log.Printf("Invalid delete-at labels:   %d", invalidLabels)
		log.Printf("Skipped (valid owner):      %d", skippedExistingUser)
		log.Printf("Skipped (missing owner):    %d", skippedMissingOwner)
		log.Printf("Skipped (invalid domain):   %d", skippedInvalidDomain)
		log.Printf("============================")
	}
}
