package stats

import "fmt"

// Stats tracks processing statistics
type Stats struct {
	TotalNamespaces      int
	Labeled              int
	Deleted              int
	LabelsRemoved        int
	InvalidLabels        int
	SkippedMissingOwner  int
	SkippedInvalidDomain int
	SkippedExistingUser  int
}

// IncTotal increments total namespaces count
func (s *Stats) IncTotal() {
	s.TotalNamespaces++
}

// IncLabeled increments labeled namespaces count
func (s *Stats) IncLabeled() {
	s.Labeled++
}

// IncDeleted increments deleted namespaces count
func (s *Stats) IncDeleted() {
	s.Deleted++
}

// IncLabelRemoved increments removed labels count
func (s *Stats) IncLabelRemoved() {
	s.LabelsRemoved++
}

// IncInvalidLabel increments invalid labels count
func (s *Stats) IncInvalidLabel() {
	s.InvalidLabels++
}

// IncSkippedMissingOwner increments missing owner skip count
func (s *Stats) IncSkippedMissingOwner() {
	s.SkippedMissingOwner++
}

// IncSkippedInvalidDomain increments invalid domain skip count
func (s *Stats) IncSkippedInvalidDomain() {
	s.SkippedInvalidDomain++
}

// IncSkippedExistingUser increments existing user skip count
func (s *Stats) IncSkippedExistingUser() {
	s.SkippedExistingUser++
}

// PrintSummary displays statistics summary
func (s *Stats) PrintSummary() {
	fmt.Println("\n============================")
	fmt.Println("Cleaner Summary")
	fmt.Println("----------------------------")
	fmt.Printf("Namespaces checked:         %d\n", s.TotalNamespaces)
	fmt.Printf("Labeled:                    %d\n", s.Labeled)
	fmt.Printf("Deleted:                    %d\n", s.Deleted)
	fmt.Printf("Labels removed:             %d\n", s.LabelsRemoved)
	fmt.Printf("Invalid labels:             %d\n", s.InvalidLabels)
	fmt.Printf("Skipped (valid owner):      %d\n", s.SkippedExistingUser)
	fmt.Printf("Skipped (missing owner):    %d\n", s.SkippedMissingOwner)
	fmt.Printf("Skipped (invalid domain):   %d\n", s.SkippedInvalidDomain)
	fmt.Println("============================")
}
