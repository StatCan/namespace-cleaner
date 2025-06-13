package cleaner

import (
	"context"
	"testing"

	clienttesting "k8s.io/client-go/testing"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCleanerDryRun(t *testing.T) {
	client := fake.NewSimpleClientset()
	cleaner := NewCleaner(true, client) // Dry-run mode

	// Test label operation
	if err := cleaner.LabelNamespace(context.TODO(), "test-ns", "2023-01-01"); err != nil {
		t.Fatalf("LabelNamespace failed: %v", err)
	}

	// Test delete operation
	if err := cleaner.DeleteNamespace(context.TODO(), "test-ns", false); err != nil {
		t.Fatalf("DeleteNamespace failed: %v", err)
	}

	// Verify no actual changes
	if ns, _ := client.CoreV1().Namespaces().Get(context.TODO(), "test-ns", metav1.GetOptions{}); ns != nil {
		t.Error("Namespace should not exist in dry-run mode")
	}
}

func TestCleanerRealOperations(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
		},
	}
	client := fake.NewSimpleClientset(ns)
	cleaner := NewCleaner(false, client) // Real mode

	// Test labeling
	if err := cleaner.LabelNamespace(context.TODO(), "test-ns", "2023-01-01"); err != nil {
		t.Fatalf("LabelNamespace failed: %v", err)
	}

	// Verify label exists
	labeledNs, _ := client.CoreV1().Namespaces().Get(context.TODO(), "test-ns", metav1.GetOptions{})
	if labeledNs.Labels[labelKey] != "2023-01-01" {
		t.Errorf("Label not applied correctly")
	}

	// Test deletion
	if err := cleaner.DeleteNamespace(context.TODO(), "test-ns", false); err != nil {
		t.Fatalf("DeleteNamespace failed: %v", err)
	}

	// Verify namespace deleted
	if _, err := client.CoreV1().Namespaces().Get(context.TODO(), "test-ns", metav1.GetOptions{}); err == nil {
		t.Error("Namespace should be deleted")
	}
}

func TestCleanerRemoveFinalizers(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-ns",
			Finalizers: []string{"test-finalizer"},
		},
	}
	client := fake.NewSimpleClientset(ns)
	cleaner := NewCleaner(false, client)

	// Test deletion with test mode (should remove finalizers)
	if err := cleaner.DeleteNamespace(context.TODO(), "test-ns", true); err != nil {
		t.Fatalf("DeleteNamespace failed: %v", err)
	}

	// Check that namespace was updated to remove finalizers
	actions := client.Actions()
	if len(actions) < 2 {
		t.Fatalf("Expected at least 2 actions, got %d", len(actions))
	}

	// The second action should be the update that removes finalizers
	updateAction, ok := actions[1].(clienttesting.UpdateAction)
	if !ok {
		t.Fatalf("Second action is not an update: %#v", actions[1])
	}

	updatedNs, ok := updateAction.GetObject().(*corev1.Namespace)
	if !ok {
		t.Fatalf("Updated object is not a namespace: %#v", updateAction.GetObject())
	}

	if len(updatedNs.Finalizers) > 0 {
		t.Errorf("Finalizers should be removed in test mode, found: %v", updatedNs.Finalizers)
	}
}