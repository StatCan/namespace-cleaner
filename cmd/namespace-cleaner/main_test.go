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

func (s *NamespaceCleanerTestSuite) TestProcessNamespaces() {
	testCases := []struct {
		name            string
		namespaces      []runtime.Object
		expectedPatches int
		expectedDeletes int
		dryRun          bool
	}{
		// Your test cases here...
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

			// Compare states and log differences
			for name, initialNS := range initialNamespaces {
				finalNS, exists := finalNamespaces[name]
				if !exists {
					s.T().Logf("Namespace %s was deleted.", name)
					continue
				}
				s.logNamespaceDiff(initialNS, finalNS)
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
