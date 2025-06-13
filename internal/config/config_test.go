package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Set up test environment
	os.Setenv("CLIENT_ID", "test-client")
	os.Setenv("CLIENT_SECRET", "test-secret")
	os.Setenv("TENANT_ID", "test-tenant")
	os.Setenv("DRY_RUN", "true")
	os.Setenv("TEST_MODE", "true")
	os.Setenv("ALLOWED_DOMAINS", "example.com,test.org")
	os.Setenv("TEST_USERS", "user1@example.com,user2@test.org")
	os.Setenv("GRACE_PERIOD", "15")
	defer func() {
		os.Unsetenv("CLIENT_ID")
		os.Unsetenv("CLIENT_SECRET")
		os.Unsetenv("TENANT_ID")
		os.Unsetenv("DRY_RUN")
		os.Unsetenv("TEST_MODE")
		os.Unsetenv("ALLOWED_DOMAINS")
		os.Unsetenv("TEST_USERS")
		os.Unsetenv("GRACE_PERIOD")
	}()

	cfg := LoadConfig()

	if cfg.ClientID != "test-client" {
		t.Errorf("Expected ClientID 'test-client', got '%s'", cfg.ClientID)
	}
	if !cfg.DryRun {
		t.Error("Expected DryRun to be true")
	}
	if len(cfg.AllowedDomains) != 2 {
		t.Errorf("Expected 2 allowed domains, got %d", len(cfg.AllowedDomains))
	}
	if cfg.GracePeriod != 15 {
		t.Errorf("Expected GracePeriod 15, got %d", cfg.GracePeriod)
	}
}

func TestGracePeriodDefaults(t *testing.T) {
	testCases := []struct {
		envValue string
		expected int
	}{
		{"", 30},
		{"30", 30},
		{"invalid", 0},
		{"-5", 0},
	}

	for _, tc := range testCases {
		t.Run(tc.envValue, func(t *testing.T) {
			os.Setenv("GRACE_PERIOD", tc.envValue)
			defer os.Unsetenv("GRACE_PERIOD")

			if got := getGracePeriod(); got != tc.expected {
				t.Errorf("Expected %d, got %d", tc.expected, got)
			}
		})
	}
}

func TestBoolEnvParsing(t *testing.T) {
	testCases := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"TRUE", true},
		{"false", false},
		{"", false},
		{"invalid", false},
	}

	for _, tc := range testCases {
		t.Run(tc.value, func(t *testing.T) {
			os.Setenv("TEST_VAR", tc.value)
			defer os.Unsetenv("TEST_VAR")

			if got := getBoolEnv("TEST_VAR", false); got != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, got)
			}
		})
	}
}
