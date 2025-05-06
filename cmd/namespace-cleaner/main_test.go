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
			for name, _ := range initialNamespaces {
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

// Test namespace labeling logic
func TestNamespaceLabeling(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			Annotations: map[string]string{
				"owner": "notfound@example.com",
			},
			Labels: map[string]string{}, // Initialize to avoid nil map
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
func TestDryRunBehavior(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dryrun-test",
			Annotations: map[string]string{
				"owner": "notfound@example.com",
			},
			Labels: map[string]string{},
		},
	}

	client := fake.NewSimpleClientset(ns)
	cfg := Config{
		TestMode:  true,
		TestUsers: []string{"test@example.com"},
		DryRun:    true,
	}

	// Run in dry-run mode
	processNamespaces(context.Background(), nil, client, cfg)

	// Validate label was not added
	updatedNs, err := client.CoreV1().Namespaces().Get(context.Background(), "dryrun-test", metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotContains(t, updatedNs.Labels, "namespace-cleaner/delete-at")
}

// Test label parsing errors
func TestLabelParsingErrors(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "invalid-label",
			Labels: map[string]string{
				"namespace-cleaner/delete-at": "invalid-date-format",
			},
			Annotations: map[string]string{
				"owner": "notfound@example.com",
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

	updatedNs, err := client.CoreV1().Namespaces().Get(context.Background(), "invalid-label", metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotContains(t, updatedNs.Labels, "namespace-cleaner/delete-at")
}

// Run the test suite
func TestMain(t *testing.T) {
	suite.Run(t, new(NamespaceCleanerTestSuite))
}
