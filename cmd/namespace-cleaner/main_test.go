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
	return time.Now().UTC().AddDate(0, 0, days).Format(labelTimeLayout)
}

// Test cases for processNamespaces
func (s *NamespaceCleanerTestSuite) TestProcessNamespaces() {
	testCases := []struct {
		name            string
		namespaces      []runtime.Object
		expectedPatches int
		expectedDeletes int
		expectedLabels  map[string]bool // namespace -> should have label
	}{
		{
			name: "Valid owner with non-existent user",
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
			name: "Expired namespace with delete-at label",
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
			name: "Namespace without delete-at label",
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
			s.client = fake.NewSimpleClientset(tc.namespaces...)
			processNamespaces(s.ctx, nil, s.client, s.config)

			// Verify namespace states
			for nsName, shouldHaveLabel := range tc.expectedLabels {
				ns, err := s.client.CoreV1().Namespaces().Get(s.ctx, nsName, metav1.GetOptions{})

				if tc.expectedDeletes > 0 && nsName == "expired-namespace" {
					require.Error(s.T(), err, "Namespace should be deleted")
					continue
				}

				require.NoError(s.T(), err)
				assert.Equal(s.T(), shouldHaveLabel,
					ns.Labels["namespace-cleaner/delete-at"] != "",
					"Label presence mismatch for %s", nsName)
			}

			// Count actions
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

// Test dry run behavior
func TestDryRunBehavior(t *testing.T) {
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

	cfg := Config{
		DryRun:         true,
		TestMode:       true,
		AllowedDomains: []string{"example.com"},
		GracePeriod:    30,
	}
	processNamespaces(context.TODO(), nil, client, cfg)

	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "dry-run-ns", metav1.GetOptions{})
	assert.Empty(t, updatedNs.Labels["namespace-cleaner/delete-at"], "Dry run should not modify labels")
}

// Test label parsing errors
func TestLabelParsingErrors(t *testing.T) {
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

	cfg := Config{
		TestMode:       true,
		AllowedDomains: []string{"example.com"},
	}
	processNamespaces(context.TODO(), nil, client, cfg)

	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "invalid-label-ns", metav1.GetOptions{})
	assert.Empty(t, updatedNs.Labels["namespace-cleaner/delete-at"], "Invalid label should be removed")
}

// Test valid namespace with active user
func TestValidNamespace(t *testing.T) {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "valid-ns",
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "kubeflow-profile",
			},
			Annotations: map[string]string{
				"owner": "test@example.com", // Exists in test users
			},
		},
	}
	client := fake.NewSimpleClientset(ns)

	cfg := Config{
		TestMode:       true,
		TestUsers:      []string{"test@example.com"},
		AllowedDomains: []string{"example.com"},
	}
	processNamespaces(context.TODO(), nil, client, cfg)

	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "valid-ns", metav1.GetOptions{})
	assert.Empty(t, updatedNs.Labels["namespace-cleaner/delete-at"], "Valid namespace should not be labeled")
}

// Run the test suite
func TestMainSuite(t *testing.T) {
	suite.Run(t, new(NamespaceCleanerTestSuite))
}
