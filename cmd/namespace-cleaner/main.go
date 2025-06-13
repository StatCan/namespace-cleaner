package main

import (
	"context"
	"time"

	"github.com/StatCan/namespace-cleaner/internal/cleaner"
	"github.com/StatCan/namespace-cleaner/internal/clients"
	"github.com/StatCan/namespace-cleaner/internal/config"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Initialize clients
	ctx := context.Background()
	graphClient := clients.NewGraphClient(cfg)
	kubeClient := clients.NewKubeClient()

	// Create cleaner based on dry-run setting
	nsCleaner := cleaner.NewCleaner(cfg.DryRun, kubeClient)

	// Execute namespace cleaning
	stats := cleaner.ProcessNamespaces(
		ctx,
		nsCleaner,
		graphClient,
		kubeClient,
		cfg,
		time.Now(),
	)

	// Print summary if in dry-run mode
	if cfg.DryRun {
		stats.PrintSummary()
	}
}