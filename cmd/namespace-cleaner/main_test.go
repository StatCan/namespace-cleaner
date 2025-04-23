package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

type NamespaceCleanerTestSuite struct {
	suite.Suite
	client    *fake.Clientset
	ctx       context.Context
	config    Config
	graceDate string
}

func TestNamespaceCleanerSuite(t *testing.T) {
	suite.Run(t, new(NamespaceCleanerTestSuite))
}

func (s *NamespaceCleanerTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.graceDate = time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	s.config = Config{
		TestMode:       true,
		TestUsers:      []string{"test@example.com"},
		AllowedDomains: []string{"example.com"},
		DryRun:         false,
		GracePeriod:    7,
	}
}

func (s *NamespaceCleanerTestSuite) printNamespaceState(header string) {
	s.T().Logf("\n=== %s ===", header)
	namespaces, _ := s.client.CoreV1().Namespaces().List(s.ctx, metav1.ListOptions{})

	for _, ns := range namespaces.Items {
		s.T().Logf("Namespace: %s", ns.Name)
		s.T().Logf("  Labels:      %v", ns.Labels)
		s.T().Logf("  Annotations: %v", ns.Annotations)
		s.T().Log("-----------------------------------")
	}
}

func (s *NamespaceCleanerTestSuite) TestProcessNamespaces() {
	now := time.Now()
	pastDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	futureDate := now.AddDate(0, 0, 1).Format("2006-01-02")

	testCases := []struct {
		name            string
		namespaces      []runtime.Object
		expectedPatches int
		expectedDeletes int
	}{
		{
			name: "Mark namespace for deletion when user doesn't exist",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-ns",
						Labels: map[string]string{
							"app.kubernetes.io/part-of": "kubeflow-profile",
						},
						Annotations: map[string]string{
							"owner": "nonexistent@example.com",
						},
					},
				},
			},
			expectedPatches: 1,
			expectedDeletes: 0,
		},
		{
			name: "Dry run doesn't modify namespaces",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-ns",
						Labels: map[string]string{
							"app.kubernetes.io/part-of": "kubeflow-profile",
						},
						Annotations: map[string]string{
							"owner": "nonexistent@example.com",
						},
					},
				},
			},
			expectedPatches: 0,
			expectedDeletes: 0,
		},
		{
			name: "Delete expired namespace when user doesn't exist",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "expired-ns",
						Labels: map[string]string{
							"namespace-cleaner/delete-at": pastDate,
						},
						Annotations: map[string]string{
							"owner": "nonexistent@example.com",
						},
					},
				},
			},
			expectedPatches: 0,
			expectedDeletes: 1,
		},
		{
			name: "Remove label when user recovers",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "recovered-ns",
						Labels: map[string]string{
							"namespace-cleaner/delete-at": futureDate,
						},
						Annotations: map[string]string{
							"owner": "test@example.com",
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
			s.printNamespaceState("Initial State")

			processNamespaces(s.ctx, nil, s.client, s.config)
			s.printNamespaceState("Final State")

			actions := s.client.Actions()
			s.T().Logf("Performed actions: %v", actions)

			patches, deletes := 0, 0
			for _, action := range actions {
				if action.Matches("patch", "namespaces") {
					patches++
				}
				if action.Matches("delete", "namespaces") {
					deletes++
				}
			}

			assert.Equal(s.T(), tc.expectedPatches, patches, "Patch count mismatch")
			assert.Equal(s.T(), tc.expectedDeletes, deletes, "Delete count mismatch")
		})
	}
}

func TestGetGracePeriod(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     int
	}{
		{"Default", "", 30},
		{"Valid", "7", 7},
		{"Invalid", "abc", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GRACE_PERIOD", tt.envValue)
			assert.Equal(t, tt.want, getGracePeriod())
		})
	}
}

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
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			assert.Equal(t, tt.want, validDomain(tt.email, tt.domains))
		})
	}
}

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
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			assert.Equal(t, tt.want, userExists(context.Background(), cfg, nil, tt.email))
		})
	}
}
