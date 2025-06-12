package cleaner

import (
	"context"
	"log"
	"time"

	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/StatCan/namespace-cleaner/internal/clients"
	"github.com/StatCan/namespace-cleaner/internal/config"
	"github.com/StatCan/namespace-cleaner/pkg/stats"
)

// ProcessNamespaces executes namespace cleaning workflow
func ProcessNamespaces(
	ctx context.Context,
	cleaner NamespaceCleaner,
	graph *msgraphsdk.GraphServiceClient,
	kube kubernetes.Interface,
	cfg *config.Config,
	now time.Time,
) *stats.Stats {
	stats := &stats.Stats{}

	_, err := kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Error listing namespaces: %v", err)
		return stats
	}

	graceDate := now.Add(time.Duration(cfg.GracePeriod) * 24 * time.Hour).Format(labelTimeLayout)

	// Phase 1: Process unlabeled namespaces
	processPhase1(ctx, cleaner, graph, kube, cfg, graceDate, stats)

	// Phase 2: Process labeled namespaces
	processPhase2(ctx, cleaner, graph, kube, cfg, now, stats)

	return stats
}

func processPhase1(
	ctx context.Context,
	cleaner NamespaceCleaner,
	graph *msgraphsdk.GraphServiceClient,
	kube kubernetes.Interface,
	cfg *config.Config,
	graceDate string,
	stats *stats.Stats,
) {
	nsList, err := kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/part-of=kubeflow-profile,!" + labelKey,
	})
	if err != nil {
		log.Fatalf("Error listing namespaces: %v", err)
	}

	for _, ns := range nsList.Items {
		stats.IncTotal()
		processUnlabeledNamespace(ctx, cleaner, graph, &ns, cfg, graceDate, stats)
	}
}

func processPhase2(
	ctx context.Context,
	cleaner NamespaceCleaner,
	graph *msgraphsdk.GraphServiceClient,
	kube kubernetes.Interface,
	cfg *config.Config,
	now time.Time,
	stats *stats.Stats,
) {
	labeledNs, err := kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: labelKey,
	})
	if err != nil {
		log.Printf("Error listing labeled namespaces: %v", err) 
	}

	for _, ns := range labeledNs.Items {
		stats.IncTotal()
		processLabeledNamespace(ctx, cleaner, graph, &ns, cfg, now, stats)
	}
}

func processUnlabeledNamespace(
	ctx context.Context,
	cleaner NamespaceCleaner,
	graph *msgraphsdk.GraphServiceClient,
	ns *corev1.Namespace,
	cfg *config.Config,
	graceDate string,
	stats *stats.Stats,
) {
	email, found := ns.Annotations["owner"]
	if !found {
		stats.IncSkippedMissingOwner()
		return
	}

	if !clients.ValidDomain(email, cfg.AllowedDomains) {
		stats.IncSkippedInvalidDomain()
		return
	}

	if clients.UserExists(ctx, cfg, graph, email) {
		stats.IncSkippedExistingUser()
		return
	}

	if err := cleaner.LabelNamespace(ctx, ns.Name, graceDate); err != nil {
		log.Printf("Error labeling %s: %v", ns.Name, err)
	} else {
		stats.IncLabeled()
	}
	if err := cleaner.RemoveLabel(ctx, ns.Name); err != nil {
		log.Printf("Error removing label from %s: %v", ns.Name, err)
	}
}

func processLabeledNamespace(
	ctx context.Context,
	cleaner NamespaceCleaner,
	graph *msgraphsdk.GraphServiceClient,
	ns *corev1.Namespace,
	cfg *config.Config,
	today time.Time,
	stats *stats.Stats,
) {
	email, found := ns.Annotations["owner"]
	if !found {
		stats.IncSkippedMissingOwner()
		return
	}

	labelValue := ns.Labels[labelKey]
	deletionDate, err := time.ParseInLocation(labelTimeLayout, labelValue, time.UTC)
	if err != nil {
		log.Printf("Invalid delete-at label in %s: %q", ns.Name, labelValue)
		stats.IncInvalidLabel()
		return
	}

	if !clients.ValidDomain(email, cfg.AllowedDomains) {
		stats.IncSkippedInvalidDomain()
		return
	}

	if clients.UserExists(ctx, cfg, graph, email) {
		if err := cleaner.RemoveLabel(ctx, ns.Name); err != nil {
			log.Printf("Error removing label: %v", err)
		} else {
			stats.IncLabelRemoved()
		}
		return
	}

	if today.After(deletionDate) {
		if err := cleaner.DeleteNamespace(ctx, ns.Name, cfg.TestMode); err != nil {
			log.Printf("Error deleting ns %s: %v", ns.Name, err)
		} else {
			stats.IncDeleted()
		}
	}
}
