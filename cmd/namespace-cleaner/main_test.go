// main_test.go
package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	// Kubernetes API types and fake client for testing processNamespaces
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	// The Graph SDK is used only in non-test mode. In our tests we exercise TestMode.
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// TestGetGracePeriod_Default verifies that if GRACE_PERIOD is not set, default is 30.
func TestGetGracePeriod_Default(t *testing.T) {
	// Ensure GRACE_PERIOD is not set
	os.Unsetenv("GRACE_PERIOD")
	if gp := getGracePeriod(); gp != 30 {
		t.Errorf("Expected default grace period 30, got %d", gp)
	}
}

// TestGetGracePeriod_Custom verifies that a set value is parsed correctly.
func TestGetGracePeriod_Custom(t *testing.T) {
	os.Setenv("GRACE_PERIOD", "45")
	defer os.Unsetenv("GRACE_PERIOD")
	if gp := getGracePeriod(); gp != 45 {
		t.Errorf("Expected grace period 45, got %d", gp)
	}
}

// TestValidDomain ensures that emails with allowed domains are valid and otherwise are not.
func TestValidDomain(t *testing.T) {
	allowed := []string{"example.com", "test.com"}
	cases := []struct {
		email string
		valid bool
	}{
		{"user@example.com", true},
		{"user@test.com", true},
		{"user@invalid.com", false},
		{"invalid-email", false},
	}

	for _, tc := range cases {
		if validDomain(tc.email, allowed) != tc.valid {
			t.Errorf("validDomain(%q) expected %v", tc.email, tc.valid)
		}
	}
}

// TestUserExists_TestMode tests the userExists function when TestMode is enabled.
// In test mode, the function simply looks for the email in the provided TestUsers list.
func TestUserExists_TestMode(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		TestMode:  true,
		TestUsers: []string{"user1@example.com", "user2@example.com"},
	}

	// In test mode the client parameter is not used.
	if exists := userExists(ctx, cfg, nil, "user1@example.com"); !exists {
		t.Errorf("Expected user1@example.com to be found in test mode")
	}
	if exists := userExists(ctx, cfg, nil, "user3@example.com"); exists {
		t.Errorf("Expected user3@example.com to not be found in test mode")
	}
}

// TestProcessNamespaces_DryRun tests processNamespaces using a fake Kubernetes client and DryRun mode.
// This test creates a couple of namespaces with different annotations and allowed domains,
// then ensures that with DryRun enabled no actual patch or deletion is attempted,
// and that log output indicates what would have been done.
func TestProcessNamespaces_DryRun(t *testing.T) {
	// Create two namespaces:
	// ns1 has an owner with an allowed domain ("example.com") but the user is not in the TestUsers list
	// ns2 has an owner with a disallowed domain.
	ns1 := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "namespace1",
			Annotations: map[string]string{"owner": "user1@example.com"},
		},
	}
	ns2 := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "namespace2",
			Annotations: map[string]string{"owner": "user2@bad.com"},
		},
	}

	// Create a fake Kubernetes client populated with ns1 and ns2.
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	kubeClient := fake.NewSimpleClientset(ns1, ns2)

	// dummyGraphClient is not used in TestMode
	var dummyGraphClient *msgraphsdk.GraphServiceClient = nil

	ctx := context.Background()

	// Set up configuration with DryRun on, TestMode enabled, and allowed domains.
	cfg := Config{
		DryRun:         true,
		TestMode:       true,
		AllowedDomains: []string{"example.com"},
		TestUsers:      []string{}, // So that user1@example.com is not considered present.
		GracePeriod:    
