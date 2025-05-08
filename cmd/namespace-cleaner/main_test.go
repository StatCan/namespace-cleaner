// main_test.go
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

type NamespaceCleanerTestSuite struct {
	suite.Suite
	client *fake.Clientset
	ctx    context.Context
	config Config
}

func (s *NamespaceCleanerTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.config = Config{
		TestMode:       true,
		TestUsers:      []string{"test@example.com"},
		AllowedDomains: []string{"example.com"},
		DryRun:         false,
		GracePeriod:    7,
	}
	log.SetOutput(io.Discard) // suppress std log, rely on t.Logf
}

func getGraceDate(days int) string {
	return time.Now().UTC().AddDate(0, 0, days).Format(labelTimeLayout)
}

func (s *NamespaceCleanerTestSuite) TestProcessNamespaces() {
	testCases := []struct {
		name            string
		namespaces      []runtime.Object
		expectedPatches int
		expectedDeletes int
		expectedLabels  map[string]bool
	}{
		{
			name: "Should label namespace with nonexistent user",
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
			name: "Should delete namespace with expired label",
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
			name: "Should add label to unlabeled namespace",
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
			s.T().Logf("📥 Input: %v", getNamespaceNames(tc.namespaces))

			initialState, _ := s.client.CoreV1().Namespaces().List(s.ctx, metav1.ListOptions{})
			s.T().Logf("🔎 Initial State (%d):", len(initialState.Items))
			for _, ns := range initialState.Items {
				s.T().Logf(" - %s: Labels=%v Annotations=%v", ns.Name, ns.Labels, ns.Annotations)
			}

			processNamespaces(s.ctx, nil, s.client, s.config)

			finalState, _ := s.client.CoreV1().Namespaces().List(s.ctx, metav1.ListOptions{})
			s.T().Logf("📊 Final State (%d):", len(finalState.Items))
			for _, ns := range finalState.Items {
				s.T().Logf(" - %s: Labels=%v Annotations=%v", ns.Name, ns.Labels, ns.Annotations)
			}

			for nsName, shouldHaveLabel := range tc.expectedLabels {
				ns, err := s.client.CoreV1().Namespaces().Get(s.ctx, nsName, metav1.GetOptions{})
				if tc.expectedDeletes > 0 {
					s.T().Logf("🧪 Deletion Check: %s (should be deleted)", nsName)
					require.Error(s.T(), err, "❌ Expected %s to be deleted", nsName)
					continue
				}
				require.NoError(s.T(), err)
				labelPresent := ns.Labels["namespace-cleaner/delete-at"] != ""
				assert.Equal(s.T(), shouldHaveLabel, labelPresent,
					"Mismatch for %s: expected label=%v", nsName, shouldHaveLabel)
			}

			var patches, deletes int
			for _, action := range s.client.Actions() {
				switch {
				case action.Matches("patch", "namespaces"):
					patches++
				case action.Matches("delete", "namespaces"):
					deletes++
				}
			}
			s.T().Logf("🔧 Actions: patches=%d deletes=%d", patches, deletes)
			assert.Equal(s.T(), tc.expectedPatches, patches)
			assert.Equal(s.T(), tc.expectedDeletes, deletes)
		})
	}
}

func getNamespaceNames(objs []runtime.Object) []string {
	var names []string
	for _, o := range objs {
		if ns, ok := o.(*v1.Namespace); ok {
			names = append(names, ns.Name)
		}
	}
	return names
}

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
	t.Logf("📥 Dry Run Initial State: %s Labels=%v", ns.Name, ns.Labels)

	cfg := Config{
		DryRun:         true,
		TestMode:       true,
		AllowedDomains: []string{"example.com"},
		GracePeriod:    30,
	}
	processNamespaces(context.TODO(), nil, client, cfg)

	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "dry-run-ns", metav1.GetOptions{})
	t.Logf("📊 Dry Run Final State: %s Labels=%v", updatedNs.Name, updatedNs.Labels)

	assert.Empty(t, updatedNs.Labels["namespace-cleaner/delete-at"],
		"Dry run should not modify labels")
}

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
	t.Logf("📥 Invalid Label Initial State: %s Labels=%v", ns.Name, ns.Labels)

	cfg := Config{
		TestMode:       true,
		AllowedDomains: []string{"example.com"},
	}
	processNamespaces(context.TODO(), nil, client, cfg)

	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "invalid-label-ns", metav1.GetOptions{})
	t.Logf("📊 Final State: %s Labels=%v", updatedNs.Name, updatedNs.Labels)

	assert.Empty(t, updatedNs.Labels["namespace-cleaner/delete-at"],
		"Invalid label should be removed")
}

func TestValidNamespace(t *testing.T) {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "valid-ns",
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "kubeflow-profile",
			},
			Annotations: map[string]string{"owner": "test@example.com"},
		},
	}
	client := fake.NewSimpleClientset(ns)
	t.Logf("📥 Valid Namespace Initial State: %s Labels=%v", ns.Name, ns.Labels)

	cfg := Config{
		TestMode:       true,
		TestUsers:      []string{"test@example.com"},
		AllowedDomains: []string{"example.com"},
	}
	processNamespaces(context.TODO(), nil, client, cfg)

	updatedNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "valid-ns", metav1.GetOptions{})
	t.Logf("📊 Final State: %s Labels=%v", updatedNs.Name, updatedNs.Labels)

	assert.Empty(t, updatedNs.Labels["namespace-cleaner/delete-at"],
		"Valid namespace should not be labeled")
}

func TestMainSuite(t *testing.T) {
	suite.Run(t, new(NamespaceCleanerTestSuite))
}
