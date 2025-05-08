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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	labelDeleteAt     = "namespace-cleaner/delete-at"
	ownerAnnotation   = "owner"
	testHeaderFormat  = "\n🏷️  TEST CASE: %s\n"
	sectionHeader     = "📦 %s\n"
	namespaceFormat   = "  ▸ Namespace: %s\n"
	labelFormat       = "    - Label: %s=%s\n"
	annotationFormat  = "    - Annotation: %s=%s\n"
	validationFormat  = "  ✓ %s\n"
	actionCountFormat = "  ↳ Expected: %d | Actual: %d\n"
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
	log.SetOutput(io.Discard)
}

func getDateOffset(days int) string {
	return time.Now().UTC().AddDate(0, 0, days).Format(labelTimeLayout)
}

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

func (s *NamespaceCleanerTestSuite) TestNamespaceLifecycle() {
	testCases := []struct {
		name            string
		namespaces      []runtime.Object
		expectedPatches int
		expectedDeletes int
		expectedLabels  map[string]bool
	}{
		{
			name: "Label namespace with missing owner",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "invalid-user",
						Labels: map[string]string{
							"app.kubernetes.io/part-of": "kubeflow-profile",
						},
						Annotations: map[string]string{
							ownerAnnotation: "notfound@example.com",
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
							labelDeleteAt:               getDateOffset(-1),
							"app.kubernetes.io/part-of": "kubeflow-profile",
						},
						Annotations: map[string]string{
							ownerAnnotation: "notfound@example.com",
						},
					},
				},
			},
			expectedPatches: 0,
			expectedDeletes: 1,
			expectedLabels:  map[string]bool{"expired-namespace": false},
		},
		{
			name: "Valid namespace should remain untouched",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "valid-ns",
						Labels: map[string]string{
							"app.kubernetes.io/part-of": "kubeflow-profile",
						},
						Annotations: map[string]string{
							ownerAnnotation: "test@example.com",
						},
					},
				},
			},
			expectedPatches: 0,
			expectedDeletes: 0,
			expectedLabels:  map[string]bool{"valid-ns": false},
		},
		{
			name: "Label parsing error should clear label",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "bad-label-ns",
						Labels: map[string]string{
							labelDeleteAt:               "not-a-date",
							"app.kubernetes.io/part-of": "kubeflow-profile",
						},
						Annotations: map[string]string{
							ownerAnnotation: "test@example.com",
						},
					},
				},
			},
			expectedPatches: 1,
			expectedDeletes: 0,
			expectedLabels:  map[string]bool{"bad-label-ns": false},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.client = fake.NewSimpleClientset(tc.namespaces...)
			s.logTestStart(tc.name)

			s.logSection("Initial State")
			list, _ := s.client.CoreV1().Namespaces().List(s.ctx, metav1.ListOptions{})
			for _, ns := range list.Items {
				s.logNamespaceDetails(ns)
			}

			processNamespaces(s.ctx, nil, s.client, s.config)

			s.logSection("Final State")
			list, _ = s.client.CoreV1().Namespaces().List(s.ctx, metav1.ListOptions{})
			for _, ns := range list.Items {
				s.logNamespaceDetails(ns)
			}

			s.logSection("Validation Results")
			for name, shouldHaveLabel := range tc.expectedLabels {
				ns, err := s.client.CoreV1().Namespaces().Get(s.ctx, name, metav1.GetOptions{})
				if tc.expectedDeletes > 0 {
					require.Error(s.T(), err, "Namespace should have been deleted")
					continue
				}
				require.NoError(s.T(), err)
				hasLabel := ns.Labels[labelDeleteAt] != ""
				assert.Equal(s.T(), shouldHaveLabel, hasLabel)
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
			s.T().Logf("Patch Operations: "+actionCountFormat, tc.expectedPatches, patches)
			s.T().Logf("Delete Operations: "+actionCountFormat, tc.expectedDeletes, deletes)
		})
	}
}

func (s *NamespaceCleanerTestSuite) TestDryRun() {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dry-run-ns",
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "kubeflow-profile",
			},
			Annotations: map[string]string{
				ownerAnnotation: "invalid@example.com",
			},
		},
	}
	s.client = fake.NewSimpleClientset(ns)
	s.config.DryRun = true

	processNamespaces(s.ctx, nil, s.client, s.config)

	updated, _ := s.client.CoreV1().Namespaces().Get(s.ctx, "dry-run-ns", metav1.GetOptions{})
	assert.Empty(s.T(), updated.Labels[labelDeleteAt], "Dry run should not apply label")
}

func TestNamespaceCleanerSuite(t *testing.T) {
	suite.Run(t, new(NamespaceCleanerTestSuite))
}
