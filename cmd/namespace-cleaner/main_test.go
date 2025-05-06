package main

import (
	"context"
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

//nolint:unused
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
			name: "Valid owner with non-existent user",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "invalid-user",
						Annotations: map[string]string{
							"owner": "notfound@example.com",
						},
						Labels: map[string]string{}, // Initialize empty labels
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
						Labels: map[string]string{}, // Initialize empty labels
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

			// Validate label state explicitly
			for name := range initialNamespaces {
				finalNS, exists := finalNamespaces[name]
				require.True(s.T(), exists, "Namespace %s should exist", name)

				// Validate label behavior
				if tc.expectedPatches > 0 {
					assert.Contains(s.T(), finalNS.Labels, "namespace-cleaner/delete-at",
						"Expected label not found in namespace %s", name)
				} else {
					assert.NotContains(s.T(), finalNS.Labels, "namespace-cleaner/delete-at",
						"Unexpected label in namespace %s", name)
				}
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

// Create a test function setup
func setupTestKubeClient(objs ...runtime.Object) *fake.Clientset {
	return fake.NewSimpleClientset(objs...)
}

// Example test case setup:
func TestNamespaceLabeling(t *testing.T) {
	// Setup test namespace without delete-at label but with part-of label
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "kubeflow-profile",
			},
			Annotations: map[string]string{
				"owner": "nonexistent@test.com",
			},
		},
	}
	client := setupTestKubeClient(ns)

	// Run processNamespaces with test config
	cfg := Config{
		TestMode:       true,
		AllowedDomains: []string{"test.com"},
		TestUsers:      []string{}, // User doesn't exist
		GracePeriod:    30,
	}
	processNamespaces(context.TODO(), nil, client, cfg)

	// Verify the label was added
	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "test-ns", metav1.GetOptions{})
	if _, ok := updatedNs.Labels["namespace-cleaner/delete-at"]; !ok {
		t.Error("Expected delete-at label to be added")
	}
}

// Test dry run behavior
func TestDryRunBehavior(t *testing.T) {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dry-run-ns",
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "kubeflow-profile",
			},
			Annotations: map[string]string{"owner": "nonexistent@test.com"},
		},
	}
	client := setupTestKubeClient(ns)

	cfg := Config{
		DryRun:      true,
		TestMode:    true,
		GracePeriod: 30,
	}
	processNamespaces(context.TODO(), nil, client, cfg)

	// Verify no changes were made
	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "dry-run-ns", metav1.GetOptions{})
	if _, ok := updatedNs.Labels["namespace-cleaner/delete-at"]; ok {
		t.Error("Dry run should not modify labels")
	}
}

// Test label parsing errors
func TestLabelParsingErrors(t *testing.T) {
	// Namespace with invalid delete-at label
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "invalid-label-ns",
			Labels: map[string]string{
				"namespace-cleaner/delete-at": "invalid-date-format",
			},
		},
	}
	client := setupTestKubeClient(ns)

	cfg := Config{TestMode: true}
	processNamespaces(context.TODO(), nil, client, cfg)

	// Label should be removed after parsing error
	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "invalid-label-ns", metav1.GetOptions{})
	if _, ok := updatedNs.Labels["namespace-cleaner/delete-at"]; ok {
		t.Error("Expected invalid delete-at label to be removed")
	}
}

func TestExpiredNamespaceDeletion(t *testing.T) {
	pastDate := time.Now().Add(-24 * time.Hour).Format(labelTimeLayout)
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "expired-ns",
			Labels: map[string]string{
				"namespace-cleaner/delete-at": pastDate,
			},
			Annotations: map[string]string{"owner": "deleted@test.com"},
		},
	}
	client := setupTestKubeClient(ns)

	cfg := Config{TestMode: true}
	processNamespaces(context.TODO(), nil, client, cfg)

	// Namespace should be deleted
	_, err := client.CoreV1().Namespaces().Get(context.TODO(), "expired-ns", metav1.GetOptions{})
	if err == nil {
		t.Error("Expected namespace to be deleted")
	}
}

// Run the test suite
func TestMain(t *testing.T) {
	suite.Run(t, new(NamespaceCleanerTestSuite))
}
