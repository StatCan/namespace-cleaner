package stats

import (
	"testing"
)

func TestStatsIncrements(t *testing.T) {
	s := &Stats{}

	// Test all increment methods
	s.IncTotal()
	s.IncLabeled()
	s.IncDeleted()
	s.IncLabelRemoved()
	s.IncInvalidLabel()
	s.IncSkippedMissingOwner()
	s.IncSkippedInvalidDomain()
	s.IncSkippedExistingUser()

	// Verify all increments
	if s.TotalNamespaces != 1 {
		t.Errorf("Expected TotalNamespaces=1, got %d", s.TotalNamespaces)
	}
	if s.Labeled != 1 {
		t.Errorf("Expected Labeled=1, got %d", s.Labeled)
	}
	if s.Deleted != 1 {
		t.Errorf("Expected Deleted=1, got %d", s.Deleted)
	}
	if s.LabelsRemoved != 1 {
		t.Errorf("Expected LabelsRemoved=1, got %d", s.LabelsRemoved)
	}
	if s.InvalidLabels != 1 {
		t.Errorf("Expected InvalidLabels=1, got %d", s.InvalidLabels)
	}
	if s.SkippedMissingOwner != 1 {
		t.Errorf("Expected SkippedMissingOwner=1, got %d", s.SkippedMissingOwner)
	}
	if s.SkippedInvalidDomain != 1 {
		t.Errorf("Expected SkippedInvalidDomain=1, got %d", s.SkippedInvalidDomain)
	}
	if s.SkippedExistingUser != 1 {
		t.Errorf("Expected SkippedExistingUser=1, got %d", s.SkippedExistingUser)
	}

	// Test multiple increments
	s.IncTotal()
	s.IncTotal()
	if s.TotalNamespaces != 3 {
		t.Errorf("Expected TotalNamespaces=3 after multiple increments, got %d", s.TotalNamespaces)
	}
}

func TestPrintSummary(t *testing.T) {
	s := &Stats{
		TotalNamespaces:      5,
		Labeled:              1,
		Deleted:              2,
		LabelsRemoved:        1,
		InvalidLabels:        1,
		SkippedMissingOwner:  1,
		SkippedInvalidDomain: 1,
		SkippedExistingUser:  1,
	}

	// Capture output or just verify no panic
	s.PrintSummary()
}

func TestZeroValueStats(t *testing.T) {
	s := &Stats{}

	// Verify all fields are zero-valued
	if s.TotalNamespaces != 0 {
		t.Errorf("Expected TotalNamespaces=0, got %d", s.TotalNamespaces)
	}
	if s.Labeled != 0 {
		t.Errorf("Expected Labeled=0, got %d", s.Labeled)
	}
	if s.Deleted != 0 {
		t.Errorf("Expected Deleted=0, got %d", s.Deleted)
	}
	if s.LabelsRemoved != 0 {
		t.Errorf("Expected LabelsRemoved=0, got %d", s.LabelsRemoved)
	}
	if s.InvalidLabels != 0 {
		t.Errorf("Expected InvalidLabels=0, got %d", s.InvalidLabels)
	}
	if s.SkippedMissingOwner != 0 {
		t.Errorf("Expected SkippedMissingOwner=0, got %d", s.SkippedMissingOwner)
	}
	if s.SkippedInvalidDomain != 0 {
		t.Errorf("Expected SkippedInvalidDomain=0, got %d", s.SkippedInvalidDomain)
	}
	if s.SkippedExistingUser != 0 {
		t.Errorf("Expected SkippedExistingUser=0, got %d", s.SkippedExistingUser)
	}

	// Print empty summary (shouldn't panic)
	s.PrintSummary()
}
