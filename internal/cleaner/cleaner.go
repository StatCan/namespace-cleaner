package cleaner

import (
	"context"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	labelTimeLayout = "2006-01-02_15-04-05Z"
	labelKey        = "namespace-cleaner/delete-at"
)

// NamespaceCleaner defines operations for namespace management
type NamespaceCleaner interface {
	LabelNamespace(ctx context.Context, nsName, graceDate string) error
	RemoveLabel(ctx context.Context, nsName string) error
	DeleteNamespace(ctx context.Context, nsName string, testMode bool) error
}

// Cleaner implements NamespaceCleaner with mode switching
type Cleaner struct {
	dryRun     bool
	kubeClient kubernetes.Interface
}

// NewCleaner creates a new cleaner instance
func NewCleaner(dryRun bool, kubeClient kubernetes.Interface) *Cleaner {
	return &Cleaner{
		dryRun:     dryRun,
		kubeClient: kubeClient,
	}
}

// LabelNamespace adds deletion label to a namespace
func (c *Cleaner) LabelNamespace(ctx context.Context, nsName, graceDate string) error {
	if c.dryRun {
		log.Printf("[DRY RUN] Would label %s with delete-at=%s", nsName, graceDate)
		return nil
	}

	patch := []byte(`{"metadata":{"labels":{"` + labelKey + `":"` + graceDate + `"}}}`)
	_, err := c.kubeClient.CoreV1().Namespaces().Patch(
		ctx, nsName, types.MergePatchType, patch, metav1.PatchOptions{},
	)
	return err
}

// RemoveLabel deletes the deletion label from a namespace
func (c *Cleaner) RemoveLabel(ctx context.Context, nsName string) error {
	if c.dryRun {
		log.Printf("[DRY RUN] Would remove delete-at label from %s", nsName)
		return nil
	}

	patch := []byte(`{"metadata":{"labels":{"` + labelKey + `":null}}}`)
	_, err := c.kubeClient.CoreV1().Namespaces().Patch(
		ctx, nsName, types.MergePatchType, patch, metav1.PatchOptions{},
	)
	return err
}

// DeleteNamespace deletes a namespace
func (c *Cleaner) DeleteNamespace(ctx context.Context, nsName string, testMode bool) error {
	if c.dryRun {
		log.Printf("[DRY RUN] Would delete namespace %s", nsName)
		return nil
	}

	if testMode {
		ns, err := c.kubeClient.CoreV1().Namespaces().Get(ctx, nsName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		nsCopy := ns.DeepCopy()
		nsCopy.Finalizers = nil
		_, err = c.kubeClient.CoreV1().Namespaces().Update(ctx, nsCopy, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}
	return c.kubeClient.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})
}
