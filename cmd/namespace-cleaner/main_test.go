package main

import (
	"context"
	"os"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/StatCan/namespace-cleaner/internal/clients"
	"github.com/StatCan/namespace-cleaner/internal/config"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

func TestMainFunction(t *testing.T) {
	// Set minimal environment
	os.Setenv("CLIENT_ID", "test")
	os.Setenv("CLIENT_SECRET", "test")
	os.Setenv("TENANT_ID", "test")
	defer func() {
		os.Unsetenv("CLIENT_ID")
		os.Unsetenv("CLIENT_SECRET")
		os.Unsetenv("TENANT_ID")
	}()

	// Save original functions and restore after test
	origGraphClient := clients.NewGraphClient
	origKubeClient := clients.NewKubeClient
	origUserExists := clients.UserExists
	defer func() {
		clients.NewGraphClient = origGraphClient
		clients.NewKubeClient = origKubeClient
		clients.UserExists = origUserExists
	}()

	// Mock client creation functions
	clients.NewGraphClient = func(cfg *config.Config) *msgraphsdk.GraphServiceClient {
		return nil // mock client
	}
	clients.NewKubeClient = func() kubernetes.Interface {
		return fake.NewSimpleClientset() // empty cluster
	}

	// Mock user exists function
	clients.UserExists = func(ctx context.Context, cfg *config.Config, client *msgraphsdk.GraphServiceClient, email string) bool {
		return true
	}

	// Run main
	main()
}
