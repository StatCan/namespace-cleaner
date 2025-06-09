package clients

import (
	"errors"
	"testing"

	"github.com/StatCan/namespace-cleaner/internal/config"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

func TestValidDomain(t *testing.T) {
	testCases := []struct {
		email    string
		domains  []string
		expected bool
	}{
		{"user@example.com", []string{"example.com"}, true},
		{"user@sub.example.com", []string{"example.com"}, true},
		{"user@example.org", []string{"example.com"}, false},
		{"invalid-email", []string{"example.com"}, false},
		{"user@example.com", []string{"example.org", "example.com"}, true},
	}

	for _, tc := range testCases {
		t.Run(tc.email, func(t *testing.T) {
			if got := ValidDomain(tc.email, tc.domains); got != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestUserExistsTestMode(t *testing.T) {
	cfg := &config.Config{
		TestMode:  true,
		TestUsers: []string{"test@example.com"},
	}

	// Test existing user
	if !UserExists(nil, cfg, nil, "test@example.com") {
		t.Error("User should exist in test mode")
	}

	// Test non-existing user
	if UserExists(nil, cfg, nil, "missing@example.com") {
		t.Error("User should not exist in test mode")
	}
}

func TestIsNotFoundError(t *testing.T) {
	// Create a mock NotFound error
	notFound := odataerrors.NewODataError()
	mainErr := odataerrors.NewMainError()
	mainErr.SetCode(ptr("NotFound"))
	notFound.SetErrorEscaped(mainErr)

	// Test ODataError
	if !isNotFoundError(notFound) {
		t.Error("Should recognize OData NotFound error")
	}

	// Test string match
	if !isNotFoundError(errors.New("user does not exist")) {
		t.Error("Should recognize 'does not exist' error")
	}

	// Test non-match
	if isNotFoundError(errors.New("other error")) {
		t.Error("Should not recognize other errors")
	}
}

func ptr(s string) *string {
	return &s
}
