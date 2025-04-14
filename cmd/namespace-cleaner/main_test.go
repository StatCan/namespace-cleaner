// main_test.go
package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	// For using a fake Kubernetes client
	fakeKube "k8s.io/client-go/kubernetes/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestGetGracePeriod_Default verifies that when no GRACE_PERIOD env var is set, the default value of 30 is returned.
func TestGetGracePeriod_Default(t *testing.T) {
	os.Unsetenv("GRACE_PERIOD")
	got := getGracePeriod()
	if got != 30 {
		t.Errorf("Expected grace period 30, got %d", got)
	}
}

// TestGetGracePeriod_FromEnv verifies that getGracePeriod returns the environment-specified value.
func TestGetGracePeriod_FromEnv(t *testing.T) {
	os.Setenv("GRACE_PERIOD", "45")
	defer os.Unsetenv("GRACE_PERIOD")
	got := getGracePeriod()
	if got != 45 {
		t.Errorf("Expected grace period 45, got %d", got)
	}
}

// TestValidDomain checks that validDomain properly validates email domains.
func TestValidDomain(t *testing.T) {
	domains := []string{"example.com", "test.com"}
	cases := []struct {
		email    string
		expected bool
	}{
		{"user@example.com", true},
		{"user@test.com", true},
		{"user@invalid.com", false},
		{"invalidemail", false},
	}
	for _, tc := range cases {
		got := validDomain(tc.email, domains)
		if got != tc.expected {
			t.Errorf("validDomain(%q, %v) = %v; want %v", tc.email, domains, got, tc.expected)
		}
	}
}

// TestUserExists_TestMode uses TestMode to avoid depending on an actual Graph client.
// In TestMode, userExists only checks if the email is in the cfg.TestUsers list.
func TestUserExists_TestMode(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		TestMode:  true,
		TestUsers: []string{"test@example.com", "hello@test.com"},
	}
	// The graph client isn't used in TestMode.
	var dummyGraphClient *msgraphsdk.GraphServiceClient

	cases := []struct {
		email    string
		expected bool
	}{
		{"test@example.com", true},
		{"notfound@example.com", false},
	}
	for _, tc := range cases {
		got := userExists(ctx, cfg, dummyGraphClient, tc.email)
		if got != tc.expected {
			t.Errorf("userExists(%q) = %v; want %v", tc.email, got, tc.expected)
		}
	}
}

// TestProcessNamespaces_DryRun demonstrates a simple test of processNamespaces using a fake Kubernetes client.
// This test creates two namespaces:
// - "test-ns-1" with a valid domain and owner (that exists in TestUsers),
// - "test-ns-2" with an invalid domain.
// In DryRun mode, the function should simply log the actions without modifying any namespaces.
func TestProcessNamespaces_DryRun(t *testing.T) {
	ctx := context.Background()

	// Create a fake kube client with two namespaces.
	kube := fakeKube.NewSimpleClientset(
		// A namespace with a valid owner (belongs to allowed domain, and exists in TestUsers)
		&metav1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-ns-1",
				Labels: map[string]string{
					"app.kubernetes.io/part-of": "kubeflow-profile",
				},
				Annotations: map[string]string{
					"owner": "test@example.com",
				},
			},
		},
		// A namespace with an invalid domain (will be skipped)
		&metav1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-ns-2",
				Labels: map[string]string{
					"app.kubernetes.io/part-of": "kubeflow-profile",
				},
				Annotations: map[string]string{
					"owner": "user@notallowed.com",
				},
			},
		},
	)

	// In TestMode, the graph client is not used.
	var dummyGraphClient *msgraphsdk.GraphServiceClient

	cfg := Config{
		TestMode:       true,
		DryRun:         true,
		AllowedDomains: []string{"example.com"},
		TestUsers:      []string{"test@example.com"},
		GracePeriod:    30,
	}

	// Call processNamespaces. Since DryRun is true, no patch or deletion should be performed.
	processNamespaces(ctx, dummyGraphClient, kube, cfg)

	// Retrieve the namespaces after processing.
	ns1, err := kube.CoreV1().Namespaces().Get(ctx, "test-ns-1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Error getting namespace 'test-ns-1': %v", err)
	}
	ns2, err := kube.CoreV1().Namespaces().Get(ctx, "test-ns-2", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Error getting namespace 'test-ns-2': %v", err)
	}

	// Since "test-ns-1" has a valid owner (and the user exists in TestUsers),
	// it should not have been marked with a deletion label.
	if label, found := ns1.Labels["namespace-cleaner/delete-at"]; found && strings.TrimSpace(label) != "" {
		t.Errorf("Expected no deletion label on 'test-ns-1', but found label: %s", label)
	}

	// "test-ns-2" has an invalid domain so processNamespaces should simply log and not patch it.
	if label, found := ns2.Labels["namespace-cleaner/delete-at"]; found && strings.TrimSpace(label) != "" {
		t.Errorf("Expected no deletion label on 'test-ns-2' (invalid domain), but found label: %s", label)
	}
}
