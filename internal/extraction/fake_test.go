package extraction

import (
	"context"
	"errors"
	"testing"
)

func TestFakeStructuredExtractorMatchesKnownFixture(t *testing.T) {
	t.Parallel()

	supplier := "Fictional Office Goods"
	description := "Paper"
	fake, err := NewFakeStructuredExtractor([]FakeFixture{{
		DocumentSHA256: testHash,
		Marker:         "INVOICEFLOW_FIXTURE:OFFICE-001",
		Proposal: Proposal{
			SupplierName: &supplier,
			LineItems:    []LineItemProposal{{Description: &description}},
		},
	}})
	if err != nil {
		t.Fatalf("NewFakeStructuredExtractor() error = %v", err)
	}

	proposal, err := fake.Extract(context.Background(), StructuredExtractionInput{
		DocumentSHA256: testHash,
		ReferenceText:  []PageText{{PageNumber: 1, Text: "INVOICEFLOW_FIXTURE:OFFICE-001"}},
	}, testLimits())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if proposal.SupplierName == nil || *proposal.SupplierName != supplier {
		t.Fatalf("SupplierName = %v, want %q", proposal.SupplierName, supplier)
	}
	if len(proposal.LineItems) != 1 || proposal.LineItems[0].Description == nil || *proposal.LineItems[0].Description != description {
		t.Fatalf("LineItems = %#v, want configured proposal", proposal.LineItems)
	}

	*proposal.SupplierName = "mutated output"
	proposal.LineItems[0].Description = nil
	again, err := fake.Extract(context.Background(), StructuredExtractionInput{
		DocumentSHA256: testHash,
		ReferenceText:  []PageText{{PageNumber: 1, Text: "INVOICEFLOW_FIXTURE:OFFICE-001"}},
	}, testLimits())
	if err != nil {
		t.Fatalf("second Extract() error = %v", err)
	}
	if again.SupplierName == nil || *again.SupplierName != supplier || again.LineItems[0].Description == nil {
		t.Fatalf("fixture was mutated through returned proposal: %#v", again)
	}
}

func TestFakeStructuredExtractorRequiresHashAndMarker(t *testing.T) {
	t.Parallel()

	fake, err := NewFakeStructuredExtractor([]FakeFixture{{
		DocumentSHA256: testHash,
		Marker:         "INVOICEFLOW_FIXTURE:OFFICE-001",
	}})
	if err != nil {
		t.Fatalf("NewFakeStructuredExtractor() error = %v", err)
	}

	tests := []struct {
		name  string
		input StructuredExtractionInput
	}{
		{
			name:  "unknown hash with configured marker",
			input: StructuredExtractionInput{DocumentSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ReferenceText: []PageText{{PageNumber: 1, Text: "INVOICEFLOW_FIXTURE:OFFICE-001"}}},
		},
		{
			name:  "known hash without configured marker",
			input: StructuredExtractionInput{DocumentSHA256: testHash, ReferenceText: []PageText{{PageNumber: 1, Text: "untrusted invoice content"}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proposal, err := fake.Extract(context.Background(), test.input, testLimits())
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}
			if proposal.SupplierName != nil || len(proposal.LineItems) != 0 || len(proposal.Diagnostics) != 1 || proposal.Diagnostics[0].Code != "fake_fixture_unmatched" {
				t.Fatalf("proposal = %#v, want deterministic empty partial proposal", proposal)
			}
		})
	}
}

func TestNewFakeStructuredExtractorRejectsAmbiguousFixtures(t *testing.T) {
	t.Parallel()

	_, err := NewFakeStructuredExtractor([]FakeFixture{
		{DocumentSHA256: testHash, Marker: "one"},
		{DocumentSHA256: testHash, Marker: "two"},
	})
	if !errors.Is(err, ErrInvalidFakeFixture) {
		t.Fatalf("NewFakeStructuredExtractor() error = %v, want ErrInvalidFakeFixture", err)
	}

	_, err = NewFakeStructuredExtractor([]FakeFixture{{DocumentSHA256: "not-a-hash", Marker: "one"}})
	if !errors.Is(err, ErrInvalidFakeFixture) {
		t.Fatalf("NewFakeStructuredExtractor() error = %v, want ErrInvalidFakeFixture", err)
	}
}

func TestFakeStructuredExtractorHonorsContextAndLimits(t *testing.T) {
	t.Parallel()

	fake, err := NewFakeStructuredExtractor(nil)
	if err != nil {
		t.Fatalf("NewFakeStructuredExtractor() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = fake.Extract(ctx, StructuredExtractionInput{DocumentSHA256: testHash}, testLimits())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Extract() error = %v, want context.Canceled", err)
	}

	_, err = fake.Extract(context.Background(), StructuredExtractionInput{
		DocumentSHA256: testHash,
		ReferenceText:  []PageText{{PageNumber: 1, Text: "too much"}, {PageNumber: 2, Text: "still allowed"}, {PageNumber: 3, Text: "not allowed"}},
	}, testLimits())
	if !errors.Is(err, ErrTooManyPages) {
		t.Fatalf("Extract() error = %v, want ErrTooManyPages", err)
	}
}
