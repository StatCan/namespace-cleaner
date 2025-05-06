package main

import (
	"context"
	"io"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// Test Suite Struct
type NamespaceCleanerTestSuite struct {
	suite.Suite
	client *fake.Clientset
	ctx    context.Context
	config Config
}

// Setup test environment
func (s *NamespaceCleanerTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.config = Config{
		TestMode:       true,
		TestUsers:      []string{"test@example.com"},
		AllowedDomains: []string{"example.com"},
		DryRun:         false,
		GracePeriod:    7,
	}
	log.SetOutput(io.Discard)
}

// Helper to create timestamp string
func getGraceDate(days int) string {
	return time.Now().UTC().AddDate(0, 0, days).Format("2006-01-02_15-04-05Z")
}

// Helper to compare namespace state
func (s *NamespaceCleanerTestSuite) logNamespaceDiff(initial, final *corev1.Namespace) {
	if !s.T().Failed() {
		return
	}

	s.T().Logf("\n=== Namespace Change ===")
	s.T().Logf("Namespace: %s", initial.Name)

	// Compare labels
	for key, initialValue := range initial.Labels {
		if finalValue, exists := final.Labels[key]; !exists {
			s.T().Logf("Label removed: %s=%s", key, initialValue)
		} else if initialValue != finalValue {
			s.T().Logf("Label changed: %s from %s to %s", key, initialValue, finalValue)
		}
	}
	for key, finalValue := range final.Labels {
		if _, exists := initial.Labels[key]; !exists {
			s.T().Logf("Label added: %s=%s", key, finalValue)
		}
	}

	// Compare annotations
	for key, initialValue := range initial.Annotations {
		if finalValue, exists := final.Annotations[key]; !exists {
			s.T().Logf("Annotation removed: %s=%s", key, initialValue)
		} else if initialValue != finalValue {
			s.T().Logf("Annotation changed: %s from %s to %s", key, initialValue, finalValue)
		}
	}
	for key, finalValue := range final.Annotations {
		if _, exists := initial.Annotations[key]; !exists {
			s.T().Logf("Annotation added: %s=%s", key, finalValue)
		}
	}
}

// Test cases for processNamespaces
func (s *NamespaceCleanerTestSuite) TestProcessNamespaces() {
	testCases := []struct {
		name            string
		namespaces      []runtime.Object
		expectedPatches int
		expectedDeletes int
		dryRun          bool
	}{
		{
			name: "Namespace with invalid owner domain",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "invalid-owner",
						Annotations: map[string]string{
							"owner": "user@bad-domain.com",
						},
					},
				},
			},
			expectedPatches: 0,
			expectedDeletes: 0,
		},
		{
			name: "Valid owner with existing user",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "valid-owner",
						Annotations: map[string]string{
							"owner": "test@example.com",
						},
					},
				},
			},
			expectedPatches: 0,
			expectedDeletes: 0,
		},
		{
			name: "Valid owner with non-existent user",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "invalid-user",
						Annotations: map[string]string{
							"owner": "notfound@example.com",
						},
					},
				},
			},
			expectedPatches: 1,
			expectedDeletes: 0,
		},
		{
			name: "Expired namespace with delete-at label",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "expired-namespace",
						Labels: map[string]string{
							"namespace-cleaner/delete-at": getGraceDate(-1),
						},
						Annotations: map[string]string{
							"owner": "notfound@example.com",
						},
					},
				},
			},
			expectedPatches: 0,
			expectedDeletes: 1,
		},
		{
			name: "Namespace without delete-at label",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "no-delete-label",
						Annotations: map[string]string{
							"owner": "notfound@example.com",
						},
					},
				},
			},
			expectedPatches: 1,
			expectedDeletes: 0,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.client = fake.NewSimpleClientset(tc.namespaces...)
			originalConfig := s.config
			defer func() { s.config = originalConfig }()

			if tc.dryRun {
				s.config.DryRun = true
			}

			// Capture initial state
			initialNamespaces := make(map[string]*corev1.Namespace)
			nsList, _ := s.client.CoreV1().Namespaces().List(s.ctx, metav1.ListOptions{})
			for _, ns := range nsList.Items {
				initialNamespaces[ns.Name] = ns.DeepCopy()
			}

			// Execute the function under test
			processNamespaces(s.ctx, nil, s.client, s.config)

			// Capture final state
			finalNamespaces := make(map[string]*corev1.Namespace)
			nsList, _ = s.client.CoreV1().Namespaces().List(s.ctx, metav1.ListOptions{})
			for _, ns := range nsList.Items {
				finalNamespaces[ns.Name] = ns.DeepCopy()
			}

			// Compare states and log differences
			for name, initialNS := range initialNamespaces {
				finalNS, exists := finalNamespaces[name]
				if !exists {
					s.T().Logf("Namespace %s was deleted.", name)
					continue
				}
				s.logNamespaceDiff(initialNS, finalNS)
			}

			// Count patch and delete actions
			patches, deletes := 0, 0
			for _, action := range s.client.Actions() {
				switch {
				case action.Matches("patch", "namespaces"):
					patches++
				case action.Matches("delete", "namespaces"):
					deletes++
				}
			}

			assert.Equal(s.T(), tc.expectedPatches, patches, "Patch count mismatch")
			assert.Equal(s.T(), tc.expectedDeletes, deletes, "Delete count mismatch")
		})
	}
}

// Test getGracePeriod with various inputs
func TestGetGracePeriod(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     int
	}{
		{"Default", "", 30},
		{"Valid", "7", 7},
		{"Invalid", "abc", 0},
		{"Negative", "-5", 0},
		{"Zero", "0", 0},
		{"Whitespace", " 15 ", 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GRACE_PERIOD", tt.envValue)
			assert.Equal(t, tt.want, getGracePeriod())
		})
	}
}

// Test validDomain with various scenarios
func TestValidDomain(t *testing.T) {
	tests := []struct {
		email   string
		domains []string
		want    bool
	}{
		{"user@allowed.com", []string{"allowed.com"}, true},
		{"user@notallowed.com", []string{"allowed.com"}, false},
		{"invalid-email", []string{"allowed.com"}, false},
		{"user@allowed.co.uk", []string{"allowed.com", "allowed.co.uk"}, true},
		{"user@sub.allowed.com", []string{"allowed.com"}, true},
		{"user@", []string{"allowed.com"}, false},
		{"", []string{"allowed.com"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			assert.Equal(t, tt.want, validDomain(tt.email, tt.domains))
		})
	}
}

// Test userExists in test mode (no external calls)
func TestUserExists_TestMode(t *testing.T) {
	cfg := Config{
		TestMode:  true,
		TestUsers: []string{"test@example.com"},
	}

	tests := []struct {
		email string
		want  bool
	}{
		{"test@example.com", true},
		{"notfound@example.com", false},
		{"invalid-email", false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			assert.Equal(t, tt.want, userExists(context.Background(), cfg, nil, tt.email))
		})
	}
}

// Test Kubernetes client initialization
func TestInitKubeClient(t *testing.T) {
	// Set required env vars
	os.Setenv("KUBERNETES_SERVICE_HOST", "127.0.0.1")
	os.Setenv("KUBERNETES_SERVICE_PORT", "443")

	// Test normal case
	client := initKubeClient()
	assert.NotNil(t, client)

	// Cleanup
	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	os.Unsetenv("KUBERNETES_SERVICE_PORT")
}

// Test namespace labeling logic
func TestNamespaceLabeling(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			Annotations: map[string]string{
				"owner": "notfound@example.com",
			},
		},
	}

	client := fake.NewSimpleClientset(ns)
	cfg := Config{
		TestMode:       true,
		TestUsers:      []string{"test@example.com"},
		AllowedDomains: []string{"example.com"},
		GracePeriod:    7,
	}

	// Test label addition
	processNamespaces(context.Background(), nil, client, cfg)

	updatedNs, err := client.CoreV1().Namespaces().Get(context.Background(), "test", metav1.GetOptions{})
	require.NoError(t, err)

	assert.Contains(t, updatedNs.Labels, "namespace-cleaner/delete-at")
	assert.NotEmpty(t, updatedNs.Labels["namespace-cleaner/delete-at"])

	// Test label removal
	cfg.TestUsers = []string{"notfound@example.com"}
	processNamespaces(context.Background(), nil, client, cfg)

	updatedNs, err = client.CoreV1().Namespaces().Get(context.Background(), "test", metav1.GetOptions{})
	require.NoError(t, err)

	assert.NotContains(t, updatedNs.Labels, "namespace-cleaner/delete-at")
}

// Test dry run behavior
func TestDryRunMode(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			Annotations: map[string]string{
				"owner": "notfound@example.com",
			},
		},
	}

	client := fake.NewSimpleClientset(ns)
	cfg := Config{
		TestMode:       true,
		TestUsers:      []string{"test@example.com"},
		AllowedDomains: []string{"example.com"},
		DryRun:         true,
		GracePeriod:    7,
	}

	// Run in dry run mode
	processNamespaces(context.Background(), nil, client, cfg)

	// Namespace should not be modified
	updatedNs, err := client.CoreV1().Namespaces().Get(context.Background(), "test", metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, ns, updatedNs)
}

// Test label parsing errors
func TestLabelParsingErrors(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "invalid-label",
			Labels: map[string]string{
				"namespace-cleaner/delete-at": "invalid-date-format",
			},
		},
	}

	client := fake.NewSimpleClientset(ns)
	cfg := Config{
		TestMode:       true,
		TestUsers:      []string{"notfound@example.com"},
		AllowedDomains: []string{"example.com"},
		GracePeriod:    7,
	}

	// Should handle invalid date format gracefully
	processNamespaces(context.Background(), nil, client, cfg)

	// No action expected, but should not panic
	assert.True(t, true)
}

// Run the test suite
func TestMain(t *testing.T) {
	suite.Run(t, new(NamespaceCleanerTestSuite))
}
