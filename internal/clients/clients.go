package clients

import (
	"context"
	"log"
	"os"
	"strings"

	msauth "github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	odataerrors "github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/StatCan/namespace-cleaner/internal/config"
)

// Make client creation functions mockable
var (
	NewGraphClient = newGraphClient
	NewKubeClient  = newKubeClient
)

// UserExists is a function variable to check if a user exists in Azure AD.
//
// It points to the defaultUserExists implementation by default, but can be
// overridden in unit tests to avoid real calls to Microsoft Graph.
//
// This enables simple function-level dependency injection for mocking behavior
// without introducing interfaces or rewriting the call sites.
//
// Example test override:
//     clients.UserExists = func(ctx context.Context, cfg *config.Config, client *msgraphsdk.GraphServiceClient, email string) bool {
//         return true // or false depending on the test case
//     }
var UserExists = defaultUserExists

func newGraphClient(cfg *config.Config) *msgraphsdk.GraphServiceClient {
	if cfg.TestMode {
		return &msgraphsdk.GraphServiceClient{}
	}

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

func newKubeClient() kubernetes.Interface {
	if cfg, err := rest.InClusterConfig(); err == nil {
		kubeClient, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			log.Fatalf("Failed to create Kubernetes client: %v", err)
		}
		return kubeClient
	}

	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		kubeClient, err := kubernetes.NewForConfig(&rest.Config{
			Host: "http://localhost:8080",
		})
		if err != nil {
			log.Fatalf("Failed to create out-of-cluster Kubernetes client: %v", err)
		}
		return kubeClient
	}

	log.Fatal("No valid Kubernetes config found")
	return nil
}

// defaultUserExists checks if a user exists in Azure AD
func defaultUserExists(ctx context.Context, cfg *config.Config, client *msgraphsdk.GraphServiceClient, email string) bool {
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
		if isNotFoundError(err) {
			return false
		}
		log.Printf("Error checking user %s: %v", email, err)
		return false
	}
	return true
}

// isNotFoundError checks if an error is a "not found" error
func isNotFoundError(err error) bool {
	if respErr, ok := err.(*odataerrors.ODataError); ok {
		if mainError := respErr.GetErrorEscaped(); mainError != nil {
			if code := mainError.GetCode(); code != nil && *code == "NotFound" {
				return true
			}
		}
	}
	return strings.Contains(err.Error(), "does not exist")
}

// ValidDomain checks if an email domain is allowed
func ValidDomain(email string, domains []string) bool {
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
