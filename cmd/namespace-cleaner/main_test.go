package main

import (
	"context"
	"io"
	"log"
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
	graceDate string // Used in test setup
}

func (s *NamespaceCleanerTestSuite) SetupTest() {
	s.ctx = context.Background()
	now := time.Now().UTC()

	// Set grace date ~7 days ahead in UTC
	s.graceDate = now.AddDate(0, 0, 7).Format(labelTimeLayout)

	// Default config for tests
	s.config = Config{
		TestMode:       true,
		TestUsers:      []string{"test@example.com"},
		AllowedDomains: []string{"example.com"},
		DryRun:         false,
		GracePeriod:    7,
	}
	log.SetOutput(io.Discard) // Silence logs
}

// Helper for printing diffs in failed tests
func (s *NamespaceCleanerTestSuite) logNamespaceDiff(initial, final *corev1.Namespace) {
	if !s.T().Failed() {
		return
	}

	s.T().Logf("\n=== Namespace Change ===")
	s.T().Logf("Namespace: %s", initial.Name)

	if len(initial.Labels) != len(final.Labels) {
		s.T().Logf("Label changes:")
		for k, v := range final.Labels {
			if initial.Labels[k] != v {
				s.T().Logf("  + %s=%s", k, v)
			}
		}
		for k := range initial.Labels {
			if _, exists := final.Labels[k]; !exists {
				s.T().Logf("  - %s", k)
			}
		}
	}
}

func (s *NamespaceCleanerTestSuite) TestProcessNamespaces() {
	now := time.Now().UTC()

	// Helper to format time in label format
	formatLabelTime := func(t time.Time) string {
		return t.UTC().Format(labelTimeLayout)
	}

	testCases := []struct {
		name            string
		namespaces      []runtime.Object
		expectedPatches int
		expectedDeletes int
		dryRun          bool
	}{
		{
			name: "Namespace should be deleted (expired)",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ns-expired",
						Labels: map[string]string{
							"app.kubernetes.io/part-of":   "kubeflow-profile",
							"namespace-cleaner/delete-at": formatLabelTime(now.AddDate(0, 0, -8)),
						},
						Annotations: map[string]string{
							"owner": "olduser@example.com",
						},
					},
				},
			},
			expectedPatches: 0,
			expectedDeletes: 1,
		},
		{
			name: "Namespace should NOT be deleted (still valid)",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ns-valid",
						Labels: map[string]string{
							"app.kubernetes.io/part-of":   "kubeflow-profile",
							"namespace-cleaner/delete-at": formatLabelTime(now.AddDate(0, 0, -5)),
						},
						Annotations: map[string]string{
							"owner": "olduser@example.com",
						},
					},
				},
			},
			expectedPatches: 0,
			expectedDeletes: 0,
		},
		{
			name: "User returned -> remove delete-at label",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ns-restored",
						Labels: map[string]string{
							"app.kubernetes.io/part-of":   "kubeflow-profile",
							"namespace-cleaner/delete-at": formatLabelTime(now.AddDate(0, 0, -8)),
						},
						Annotations: map[string]string{
							"owner": "test@example.com", // Matches TestUsers
						},
					},
				},
			},
			expectedPatches: 1,
			expectedDeletes: 0,
		},
		{
			name: "Dry run prevents deletion or patch",
			namespaces: []runtime.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ns-dryrun",
						Labels: map[string]string{
							"app.kubernetes.io/part-of":   "kubeflow-profile",
							"namespace-cleaner/delete-at": formatLabelTime(now.AddDate(0, 0, -8)),
						},
						Annotations: map[string]string{
							"owner": "olduser@example.com",
						},
					},
				},
			},
			expectedPatches: 0,
			expectedDeletes: 0,
			dryRun:          true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Initialize fake client with test objects
			s.client = fake.NewSimpleClientset(tc.namespaces...)
			originalConfig := s.config
			defer func() { s.config = originalConfig }() // Reset config after test

			if tc.dryRun {
				s.config.DryRun = true
			}

			// Capture initial state
			initialNS, _ := s.client.CoreV1().Namespaces().Get(s.ctx, "ns-expired", metav1.GetOptions{})
			if initialNS == nil {
				initialNS, _ = s.client.CoreV1().Namespaces().Get(s.ctx, "ns-valid", metav1.GetOptions{})
			}
			if initialNS == nil {
				initialNS, _ = s.client.CoreV1().Namespaces().Get(s.ctx, "ns-restored", metav1.GetOptions{})
			}
			if initialNS == nil {
				initialNS, _ = s.client.CoreV1().Namespaces().Get(s.ctx, "ns-dryrun", metav1.GetOptions{})
			}

			// Execute test
			processNamespaces(s.ctx, nil, s.client, s.config)

			// Check post-processing state
			var finalNS *corev1.Namespace
			if initialNS != nil {
				finalNS, _ = s.client.CoreV1().Namespaces().Get(s.ctx, initialNS.Name, metav1.GetOptions{})
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

			// Log diff if failure occurred
			if s.T().Failed() && initialNS != nil && finalNS != nil {
				s.logNamespaceDiff(initialNS, finalNS)
				s.T().Logf("Actions performed: %d patches, %d deletes", patches, deletes)
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
