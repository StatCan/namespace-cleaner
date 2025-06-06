package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds application configuration
type Config struct {
	ClientID       string
	ClientSecret   string
	TenantID       string
	DryRun         bool
	TestMode       bool
	AllowedDomains []string
	TestUsers      []string
	GracePeriod    int
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	return &Config{
		ClientID:       os.Getenv("CLIENT_ID"),
		ClientSecret:   os.Getenv("CLIENT_SECRET"),
		TenantID:       os.Getenv("TENANT_ID"),
		DryRun:         getBoolEnv("DRY_RUN", false),
		TestMode:       getBoolEnv("TEST_MODE", false),
		AllowedDomains: splitEnv("ALLOWED_DOMAINS"),
		TestUsers:      splitEnv("TEST_USERS"),
		GracePeriod:    getGracePeriod(),
	}
}

// getBoolEnv parses a boolean environment variable
func getBoolEnv(key string, defaultValue bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	return strings.ToLower(val) == "true"
}

// splitEnv splits a comma-separated environment variable
func splitEnv(key string) []string {
	val := os.Getenv(key)
	if val == "" {
		return []string{}
	}
	return strings.Split(val, ",")
}

// getGracePeriod parses GRACE_PERIOD environment variable
func getGracePeriod() int {
	val := os.Getenv("GRACE_PERIOD")
	if val == "" {
		return 30
	}

	days, err := strconv.Atoi(val)
	if err != nil || days < 0 {
		return 0
	}
	return days
}
