// main_test.go
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// NamespaceCleanerTestSuite defines the test suite for namespace cleaner functionality
type NamespaceCleanerTestSuite struct {
	suite.Suite
	client *fake.Clientset
	ctx    context.Context
	config Config
}

// Test logging constants and helpers
const (
	testHeaderFormat  = "\n🏷️  TEST CASE: %s\n"
	sectionHeader     = "📦 %s\n"
	namespaceFormat   = "  ▸ Namespace: %s\n"
	labelFormat       = "    - Label: %s=%s\n"
	annotationFormat  = "    - Annotation: %s=%s\n"
	validationFormat  = "  ✓ %s\n"
	actionCountFormat = "  ↳ Expected: %d | Actual: %d\n"
)

func (s *NamespaceCleanerTestSuite) logTestStart(name string) {
	s.T().Logf(testHeaderFormat, name)
}

func (s *NamespaceCleanerTestSuite) logSection(title string) {
	s.T().Logf(sectionHeader, title)
}

func (s *NamespaceCleanerTestSuite) logNamespaceDetails(ns corev1.Namespace) {
	s.T().Logf(namespaceFormat, ns.Name)
	for k, v := range ns.Labels {
		s.T().Logf(labelFormat, k, v)
	}
	for k, v := range ns.Annotations {
		s.T().Logf(annotationFormat, k, v)
	}
}

// SetupTest initializes the test environment before each test case
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

// getGraceDate generates a timestamp string for testing label dates
func getGraceDate(days int) string {
	return time.Now().UTC().AddDate(0, 0, days).Format(labelTimeLayout)
}

// TestProcessNamespaces contains test cases for namespace processing logic
func (s *NamespaceCleanerTestSuite) TestProcessNamespaces() {
	testCases := []struct {
		name            string
		namespaces      []runtime.Object
		expectedPatches int
		expectedDeletes int
		expectedLabels  map[string]bool
	}{
		{
			name: "Label namespace with non-existent owner",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "invalid-user",
						Labels: map[string]string{
							"app.kubernetes.io/part-of": "kubeflow-profile",
						},
						Annotations: map[string]string{
							"owner": "notfound@example.com",
						},
					},
				},
			},
			expectedPatches: 1,
			expectedDeletes: 0,
			expectedLabels:  map[string]bool{"invalid-user": true},
		},
		{
			name: "Delete namespace with expired label",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "expired-namespace",
						Labels: map[string]string{
							"namespace-cleaner/delete-at": getGraceDate(-1),
							"app.kubernetes.io/part-of":   "kubeflow-profile",
						},
						Annotations: map[string]string{
							"owner": "notfound@example.com",
						},
					},
				},
			},
			expectedPatches: 0,
			expectedDeletes: 1,
			expectedLabels:  map[string]bool{"expired-namespace": false},
		},
		{
			name: "Add label to unlabeled namespace",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "no-delete-label",
						Labels: map[string]string{
							"app.kubernetes.io/part-of": "kubeflow-profile",
						},
						Annotations: map[string]string{
							"owner": "notfound@example.com",
						},
					},
				},
			},
			expectedPatches: 1,
			expectedDeletes: 0,
			expectedLabels:  map[string]bool{"no-delete-label": true},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Initialize test environment
			s.client = fake.NewSimpleClientset(tc.namespaces...)
			s.logTestStart(tc.name)

			// Log configuration
			s.logSection("Test Configuration")
			s.T().Logf("  - Dry Run: %t", s.config.DryRun)
			s.T().Logf("  - Grace Period: %d days", s.config.GracePeriod)
			s.T().Logf("  - Allowed Domains: %v", s.config.AllowedDomains)

			// Log initial state
			s.logSection("Initial State")
			initialState, _ := s.client.CoreV1().Namespaces().List(s.ctx, metav1.ListOptions{})
			for _, ns := range initialState.Items {
				s.logNamespaceDetails(ns)
			}

			// Process namespaces
			processNamespaces(s.ctx, nil, s.client, s.config)

			// Log final state
			s.logSection("Final State")
			finalState, _ := s.client.CoreV1().Namespaces().List(s.ctx, metav1.ListOptions{})
			if len(finalState.Items) == 0 {
				s.T().Log("  No namespaces remaining")
			}
			for _, ns := range finalState.Items {
				s.logNamespaceDetails(ns)
			}

			// Validate results
			s.logSection("Validation Results")
			for nsName, shouldHaveLabel := range tc.expectedLabels {
				ns, err := s.client.CoreV1().Namespaces().Get(s.ctx, nsName, metav1.GetOptions{})

				if tc.expectedDeletes > 0 {
					require.Error(s.T(), err, "Namespace should be deleted")
					s.T().Logf(validationFormat, fmt.Sprintf("Namespace %s was deleted", nsName))
					continue
				}

				require.NoError(s.T(), err)
				labelExists := ns.Labels["namespace-cleaner/delete-at"] != ""
				assert.Equal(s.T(), shouldHaveLabel, labelExists)
				s.T().Logf(validationFormat, fmt.Sprintf("Namespace %s label state correct", nsName))
			}

			// Verify action counts
			var patches, deletes int
			for _, action := range s.client.Actions() {
				switch {
				case action.Matches("patch", "namespaces"):
					patches++
				case action.Matches("delete", "namespaces"):
					deletes++
				}
			}
			s.T().Logf("Patch Operations: "+actionCountFormat, tc.expectedPatches, patches)
			s.T().Logf("Delete Operations: "+actionCountFormat, tc.expectedDeletes, deletes)
		})
	}
}

// TestDryRunBehavior verifies that no modifications are made when DryRun is enabled
func TestDryRunBehavior(t *testing.T) {
	// Setup test namespace
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dry-run-ns",
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "kubeflow-profile",
			},
			Annotations: map[string]string{"owner": "valid@example.com"},
		},
	}
	client := fake.NewSimpleClientset(ns)

	t.Log("\n🔧 TEST: Dry Run Behavior")
	t.Log("📋 Initial State:")
	t.Logf("  - Namespace: %s", ns.Name)
	t.Logf("    Labels: %v", ns.Labels)

	cfg := Config{
		DryRun:         true,
		TestMode:       true,
		AllowedDomains: []string{"example.com"},
		GracePeriod:    30,
	}
	processNamespaces(context.TODO(), nil, client, cfg)

	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "dry-run-ns", metav1.GetOptions{})
	t.Log("\n✅ Final State:")
	t.Logf("  - Namespace: %s", updatedNs.Name)
	t.Logf("    Labels: %v", updatedNs.Labels)

	assert.Empty(t, updatedNs.Labels["namespace-cleaner/delete-at"], "Dry run should not modify labels")
}

// TestLabelParsingErrors validates handling of invalid time format labels
func TestLabelParsingErrors(t *testing.T) {
	// Setup namespace with invalid label
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "invalid-label-ns",
			Labels: map[string]string{
				"namespace-cleaner/delete-at": "invalid-date-format",
				"app.kubernetes.io/part-of":   "kubeflow-profile",
			},
			Annotations: map[string]string{"owner": "valid@example.com"},
		},
	}
	client := fake.NewSimpleClientset(ns)

	t.Log("\n🔧 TEST: Label Parsing Error Handling")
	t.Log("📋 Initial State:")
	t.Logf("  - Namespace: %s", ns.Name)
	t.Logf("    Labels: %v", ns.Labels)

	cfg := Config{
		TestMode:       true,
		AllowedDomains: []string{"example.com"},
	}
	processNamespaces(context.TODO(), nil, client, cfg)

	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "invalid-label-ns", metav1.GetOptions{})
	t.Log("\n✅ Final State:")
	t.Logf("  - Namespace: %s", updatedNs.Name)
	t.Logf("    Labels: %v", updatedNs.Labels)

	assert.Empty(t, updatedNs.Labels["namespace-cleaner/delete-at"], "Invalid label should be removed")
}

// TestValidNamespace ensures valid namespaces remain unmodified
func TestValidNamespace(t *testing.T) {
	// Setup valid namespace configuration
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "valid-ns",
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "kubeflow-profile",
			},
			Annotations: map[string]string{
				"owner": "test@example.com",
			},
		},
	}
	client := fake.NewSimpleClientset(ns)

	t.Log("\n🔧 TEST: Valid Namespace Handling")
	t.Log("📋 Initial State:")
	t.Logf("  - Namespace: %s", ns.Name)
	t.Logf("    Labels: %v", ns.Labels)

	cfg := Config{
		TestMode:       true,
		TestUsers:      []string{"test@example.com"},
		AllowedDomains: []string{"example.com"},
	}
	processNamespaces(context.TODO(), nil, client, cfg)

	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "valid-ns", metav1.GetOptions{})
	t.Log("\n✅ Final State:")
	t.Logf("  - Namespace: %s", updatedNs.Name)
	t.Logf("    Labels: %v", updatedNs.Labels)

	assert.Empty(t, updatedNs.Labels["namespace-cleaner/delete-at"], "Valid namespace should not be labeled")
}

// TestMainSuite executes the test suite
func TestMainSuite(t *testing.T) {
	suite.Run(t, new(NamespaceCleanerTestSuite))
}
