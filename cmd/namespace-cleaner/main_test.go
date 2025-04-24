package main

import (
	"context"
	"io/ioutil"
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
	graceDate string
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
	// Suppress application logs during tests
	log.SetOutput(ioutil.Discard)
}

func (s *NamespaceCleanerTestSuite) logNamespaceDiff(initial, final *corev1.Namespace) {
	if !s.T().Failed() {
		return // Only show diffs if test failed
	}

	s.T().Logf("\n=== Namespace Change ===")
	s.T().Logf("Namespace: %s", initial.Name)

	// Log label changes
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
	testCases := []struct {
		name            string
		namespaces      []runtime.Object
		expectedPatches int
		expectedDeletes int
		dryRun          bool // Specific config for test case
	}{
		{
			name: "Mark for deletion when user missing",
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
			name: "Dry run no modifications",
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
			dryRun:          true,
		},
		// ... other test cases ...
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Setup test environment
			s.client = fake.NewSimpleClientset(tc.namespaces...)
			originalConfig := s.config
			defer func() { s.config = originalConfig }() // Reset config after test

			if tc.dryRun {
				s.config.DryRun = true
			}

			// Capture initial state
			initialNS, _ := s.client.CoreV1().Namespaces().Get(s.ctx, "test-ns", metav1.GetOptions{})

			// Execute test
			processNamespaces(s.ctx, nil, s.client, s.config)

			// Capture final state
			finalNS, _ := s.client.CoreV1().Namespaces().Get(s.ctx, "test-ns", metav1.GetOptions{})

			// Calculate actions
			actions := s.client.Actions()
			patches, deletes := 0, 0
			for _, action := range actions {
				switch {
				case action.Matches("patch", "namespaces"):
					patches++
				case action.Matches("delete", "namespaces"):
					deletes++
				}
			}

			// Report diffs if test failed
			if s.T().Failed() {
				s.logNamespaceDiff(initialNS, finalNS)
				s.T().Logf("Actions performed: %d patches, %d deletes", patches, deletes)
			}

			// Assertions
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
