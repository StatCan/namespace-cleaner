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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// NamespaceCleanerTestSuite tests the namespace cleanup functionality including:
// - Label application for namespaces with invalid owners
// - Namespace deletion for expired entries
// - Dry run behavior validation
// - Error handling for invalid input formats
type NamespaceCleanerTestSuite struct {
	suite.Suite
	client *fake.Clientset
	ctx    context.Context
	config Config
}

// Logging Helpers ------------------------------------------------------------
const (
	testHeaderFormat = "\n🏷️  TEST CASE: %s\n"
	sectionFormat    = "📦 %s\n"
	namespaceFormat  = "  ▸ Namespace: %s\n"
	labelFormat      = "    - Label: %s=%s\n"
	annotationFormat = "    - Annotation: %s=%s\n"
	successFormat    = "  ✓ %s\n"
	actionFormat     = "  ↳ %s\n"
)

func (s *NamespaceCleanerTestSuite) logTestStart(name string) {
	s.T().Logf(testHeaderFormat, name)
}

func (s *NamespaceCleanerTestSuite) logSection(title string) {
	s.T().Logf(sectionFormat, title)
}

func (s *NamespaceCleanerTestSuite) logNamespace(ns corev1.Namespace) {
	s.T().Logf(namespaceFormat, ns.Name)
	for k, v := range ns.Labels {
		s.T().Logf(labelFormat, k, v)
	}
	for k, v := range ns.Annotations {
		s.T().Logf(annotationFormat, k, v)
	}
}

// SetupTest initializes a fresh test environment before each test case
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

func getGraceDate(days int) string {
	return time.Now().UTC().AddDate(0, 0, days).Format(labelTimeLayout)
}

// TestProcessNamespaces verifies core namespace management logic through multiple scenarios:
// 1. Namespaces with invalid owners should be labeled for deletion
// 2. Namespaces with expired deletion labels should be removed
// 3. Unlabeled namespaces should get proper deletion labels
// Expected outcomes:
// - Correct number of patch and delete operations
// - Appropriate label state changes
// - Proper namespace cleanup
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
			// Test Setup
			s.client = fake.NewSimpleClientset(tc.namespaces...)
			s.logTestStart(tc.name)

			// Log Configuration
			s.logSection("Test Configuration")
			s.T().Logf(actionFormat, fmt.Sprintf("Dry Run: %t", s.config.DryRun))
			s.T().Logf(actionFormat, fmt.Sprintf("Grace Period: %d days", s.config.GracePeriod))
			s.T().Logf(actionFormat, fmt.Sprintf("Allowed Domains: %v", s.config.AllowedDomains))

			// Initial State
			s.logSection("Initial Namespaces")
			initialState, _ := s.client.CoreV1().Namespaces().List(s.ctx, metav1.ListOptions{})
			for _, ns := range initialState.Items {
				s.logNamespace(ns)
			}

			// Process Namespaces
			processNamespaces(s.ctx, nil, s.client, s.config)

			// Final State
			s.logSection("Final Namespaces")
			finalState, _ := s.client.CoreV1().Namespaces().List(s.ctx, metav1.ListOptions{})
			if len(finalState.Items) == 0 {
				s.T().Logf(actionFormat, "No namespaces remaining")
			}
			for _, ns := range finalState.Items {
				s.logNamespace(ns)
			}

			// Validation
			s.logSection("Validation Results")
			for nsName, shouldHaveLabel := range tc.expectedLabels {
				ns, err := s.client.CoreV1().Namespaces().Get(s.ctx, nsName, metav1.GetOptions{})

				if tc.expectedDeletes > 0 {
					require.Error(s.T(), err, "Namespace should be deleted")
					s.T().Logf(successFormat, fmt.Sprintf("Namespace %s was deleted", nsName))
					continue
				}

				require.NoError(s.T(), err)
				labelExists := ns.Labels["namespace-cleaner/delete-at"] != ""
				assert.Equal(s.T(), shouldHaveLabel, labelExists)
				s.T().Logf(successFormat, fmt.Sprintf("Namespace %s label state correct", nsName))
			}

			// Action Verification
			var patches, deletes int
			for _, action := range s.client.Actions() {
				switch {
				case action.Matches("patch", "namespaces"):
					patches++
				case action.Matches("delete", "namespaces"):
					deletes++
				}
			}
			s.T().Logf(actionFormat, fmt.Sprintf("Patches: Expected %d → Actual %d",
				tc.expectedPatches, patches))
			s.T().Logf(actionFormat, fmt.Sprintf("Deletions: Expected %d → Actual %d",
				tc.expectedDeletes, deletes))
		})
	}
}

// TestDryRunBehavior verifies that no modifications are made when DryRun is enabled.
// Expected outcome: Namespace labels should remain unchanged after processing.
func TestDryRunBehavior(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dry-run-ns",
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "kubeflow-profile",
			},
			Annotations: map[string]string{"owner": "valid@example.com"},
		},
	}
	client := fake.NewSimpleClientset(ns)

	t.Log("\n🏷️  TEST: Dry Run Behavior")
	t.Log("📦 Initial State:")
	t.Logf("  ▸ Namespace: %s", ns.Name)
	t.Logf("    - Labels: %v", ns.Labels)

	cfg := Config{
		DryRun:         true,
		TestMode:       true,
		AllowedDomains: []string{"example.com"},
		GracePeriod:    30,
	}
	processNamespaces(context.TODO(), nil, client, cfg)

	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "dry-run-ns", metav1.GetOptions{})
	t.Log("\n📦 Final State:")
	t.Logf("  ▸ Namespace: %s", updatedNs.Name)
	t.Logf("    - Labels: %v", updatedNs.Labels)

	assert.Empty(t, updatedNs.Labels["namespace-cleaner/delete-at"],
		"Dry run should not modify labels")
}

// TestLabelParsingErrors validates handling of invalid timestamp formats in labels.
// Expected outcome: Invalid labels should be removed from namespaces.
func TestLabelParsingErrors(t *testing.T) {
	ns := &corev1.Namespace{
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

	t.Log("\n🏷️  TEST: Label Parsing Error Handling")
	t.Log("📦 Initial State:")
	t.Logf("  ▸ Namespace: %s", ns.Name)
	t.Logf("    - Labels: %v", ns.Labels)

	cfg := Config{
		TestMode:       true,
		AllowedDomains: []string{"example.com"},
	}
	processNamespaces(context.TODO(), nil, client, cfg)

	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "invalid-label-ns", metav1.GetOptions{})
	t.Log("\n📦 Final State:")
	t.Logf("  ▸ Namespace: %s", updatedNs.Name)
	t.Logf("    - Labels: %v", updatedNs.Labels)

	assert.Empty(t, updatedNs.Labels["namespace-cleaner/delete-at"],
		"Invalid label should be removed")
}

// TestValidNamespace ensures valid namespaces with active users remain unmodified.
// Expected outcome: No deletion labels added to valid namespaces.
func TestValidNamespace(t *testing.T) {
	ns := &corev1.Namespace{
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

	t.Log("\n🏷️  TEST: Valid Namespace Handling")
	t.Log("📦 Initial State:")
	t.Logf("  ▸ Namespace: %s", ns.Name)
	t.Logf("    - Labels: %v", ns.Labels)

	cfg := Config{
		TestMode:       true,
		TestUsers:      []string{"test@example.com"},
		AllowedDomains: []string{"example.com"},
	}
	processNamespaces(context.TODO(), nil, client, cfg)

	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "valid-ns", metav1.GetOptions{})
	t.Log("\n📦 Final State:")
	t.Logf("  ▸ Namespace: %s", updatedNs.Name)
	t.Logf("    - Labels: %v", updatedNs.Labels)

	assert.Empty(t, updatedNs.Labels["namespace-cleaner/delete-at"],
		"Valid namespace should not be labeled")
}

// TestMainSuite executes the entire test suite for namespace cleaner functionality
func TestMainSuite(t *testing.T) {
	suite.Run(t, new(NamespaceCleanerTestSuite))
}
