package cleaner

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/StatCan/namespace-cleaner/internal/clients"
	"github.com/StatCan/namespace-cleaner/internal/config"
	"github.com/StatCan/namespace-cleaner/pkg/stats"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// MockUserExists creates a mock for the UserExists function
func MockUserExists(result bool) func() {
	original := clients.UserExists
	clients.UserExists = func(ctx context.Context, cfg *config.Config, client *msgraphsdk.GraphServiceClient, email string) bool {
		return result
	}
	return func() { clients.UserExists = original }
}

func TestProcessUnlabeledNamespace(t *testing.T) {
	// Setup test namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
			Annotations: map[string]string{
				"owner": "user@example.com",
			},
		},
	}

	// Setup mock - user doesn't exist
	restore := MockUserExists(false)
	defer restore()

	cleaner := &mockCleaner{}
	stats := &stats.Stats{}
	cfg := &config.Config{
		AllowedDomains: []string{"example.com"},
	}

	processUnlabeledNamespace(
		context.TODO(),
		cleaner,
		nil, // graph client
		ns,
		cfg,
		"2023-01-01",
		stats,
	)

	// Verify actions
	if len(cleaner.labeled) != 1 || cleaner.labeled[0] != "test-ns" {
		t.Error("Namespace should be labeled")
	}
	if stats.Labeled != 1 {
		t.Error("Stats should show 1 labeled namespace")
	}
}

func TestProcessLabeledNamespace(t *testing.T) {
	now := time.Now()
	pastDate := now.Add(-24 * time.Hour).Format(labelTimeLayout)

	// Setup test namespace with expired label
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
			Annotations: map[string]string{
				"owner": "user@example.com",
			},
			Labels: map[string]string{
				labelKey: pastDate,
			},
		},
	}

	// Setup mock - user doesn't exist
	restore := MockUserExists(false)
	defer restore()

	cleaner := &mockCleaner{}
	stats := &stats.Stats{}
	cfg := &config.Config{
		AllowedDomains: []string{"example.com"},
	}

	processLabeledNamespace(
		context.TODO(),
		cleaner,
		nil, // graph client
		ns,
		cfg,
		now,
		stats,
	)

	// Verify actions
	if len(cleaner.deleted) != 1 || cleaner.deleted[0] != "test-ns" {
		t.Error("Namespace should be deleted")
	}
	if stats.Deleted != 1 {
		t.Error("Stats should show 1 deleted namespace")
	}
}

func TestProcessNamespaces(t *testing.T) {
	// Setup test namespaces
	unlabeledNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "unlabeled",
			Annotations: map[string]string{
				"owner": "user@example.com",
			},
		},
	}
	labeledNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "labeled",
			Annotations: map[string]string{
				"owner": "user@example.com",
			},
			Labels: map[string]string{
				labelKey: "2023-01-01_00-00-00Z",
			},
		},
	}

	// Create fake client with namespaces
	client := fake.NewSimpleClientset(unlabeledNs, labeledNs)

	// Setup mock cleaner and user check
	cleaner := &mockCleaner{}

	// Mock user doesn't exist
	restore := MockUserExists(false)
	defer restore()

	cfg := &config.Config{
		AllowedDomains: []string{"example.com"},
		GracePeriod:    30,
	}

	stats := ProcessNamespaces(
		context.TODO(),
		cleaner,
		nil, // graph client
		client,
		cfg,
	)

	// Verify stats
	if stats.TotalNamespaces != 2 {
		t.Errorf("Expected 2 namespaces, got %d", stats.TotalNamespaces)
	}
	if stats.Labeled != 1 {
		t.Error("Expected 1 namespace to be labeled")
	}
	if stats.Deleted != 1 {
		t.Error("Expected 1 namespace to be deleted")
	}
}

// Helper struct for testing
type mockCleaner struct {
	labeled       []string
	deleted       []string
	labelsRemoved []string
}

func (m *mockCleaner) LabelNamespace(ctx context.Context, nsName, graceDate string) error {
	m.labeled = append(m.labeled, nsName)
	return nil
}

func (m *mockCleaner) RemoveLabel(ctx context.Context, nsName string) error {
	m.labelsRemoved = append(m.labelsRemoved, nsName)
	return nil
}

func (m *mockCleaner) DeleteNamespace(ctx context.Context, nsName string, testMode bool) error {
	m.deleted = append(m.deleted, nsName)
	return nil
}
