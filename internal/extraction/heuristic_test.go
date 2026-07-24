package extraction

import (
	"context"
	"strings"
	"testing"
)

const heuristicTestHash = "1111111111111111111111111111111111111111111111111111111111111111"

func heuristicInput(text string) StructuredExtractionInput {
	return StructuredExtractionInput{
		DocumentSHA256: heuristicTestHash,
		ReferenceText:  []PageText{{PageNumber: 1, Text: text}},
	}
}

func extractHeuristic(t *testing.T, text string) Proposal {
	t.Helper()
	proposal, err := HeuristicStructuredExtractor{}.Extract(context.Background(), heuristicInput(text), DefaultLimits())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	return proposal
}

func value(t *testing.T, candidate *string) string {
	t.Helper()
	if candidate == nil {
		return ""
	}
	return *candidate
}

const ordinaryInvoice = `Northwind Trading GmbH
billing@northwind.example

Invoice No: NW-2291
Invoice Date: 2026-05-04
Due Date: 2026-06-03
Currency: EUR

Description               Qty   Unit price   Amount
Recycled copier paper       4        12.50      50.00
Whiteboard markers          3         7.00      21.00

Subtotal                                        71.00
VAT (19%)                                       13.49
Total due                                       84.49
`

func TestHeuristicReadsOrdinaryInvoice(t *testing.T) {
	proposal := extractHeuristic(t, ordinaryInvoice)

	for _, testCase := range []struct{ field, got, want string }{
		{"supplier_name", value(t, proposal.SupplierName), "Northwind Trading GmbH"},
		{"supplier_email", value(t, proposal.SupplierEmail), "billing@northwind.example"},
		{"invoice_number", value(t, proposal.InvoiceNumber), "NW-2291"},
		{"issue_date", value(t, proposal.IssueDate), "2026-05-04"},
		{"due_date", value(t, proposal.DueDate), "2026-06-03"},
		{"currency", value(t, proposal.Currency), "EUR"},
		{"subtotal", value(t, proposal.Subtotal), "71.00"},
		{"tax_amount", value(t, proposal.TaxAmount), "13.49"},
		{"total", value(t, proposal.Total), "84.49"},
	} {
		if testCase.got != testCase.want {
			t.Errorf("%s = %q, want %q", testCase.field, testCase.got, testCase.want)
		}
	}

	if len(proposal.LineItems) != 2 {
		t.Fatalf("line items = %d, want 2", len(proposal.LineItems))
	}
	first := proposal.LineItems[0]
	if value(t, first.Description) != "Recycled copier paper" || value(t, first.Quantity) != "4" ||
		value(t, first.UnitPrice) != "12.50" || value(t, first.Total) != "50.00" {
		t.Errorf("unexpected first line item: %+v", first)
	}
	// Per-line tax is unknown, not zero: asserting zero would make the
	// normalizer's line arithmetic check pass or fail on invented data.
	if first.TaxAmount != nil {
		t.Errorf("line tax = %q, want unset", *first.TaxAmount)
	}
}

func TestHeuristicNeverInventsMissingValues(t *testing.T) {
	proposal := extractHeuristic(t, "Just a note.\nNothing resembling an invoice here.\n")

	if proposal.HasCandidates() {
		t.Fatalf("expected no candidates, got %+v", proposal)
	}
	for name, candidate := range map[string]*string{
		"invoice_number": proposal.InvoiceNumber, "issue_date": proposal.IssueDate,
		"due_date": proposal.DueDate, "currency": proposal.Currency,
		"subtotal": proposal.Subtotal, "tax_amount": proposal.TaxAmount, "total": proposal.Total,
	} {
		if candidate != nil {
			t.Errorf("%s = %q, want nil rather than an invented value", name, *candidate)
		}
	}
	if len(proposal.Diagnostics) != 1 || proposal.Diagnostics[0].Code != HeuristicDiagnosticCode {
		t.Errorf("diagnostics = %+v, want one %s", proposal.Diagnostics, HeuristicDiagnosticCode)
	}
}

func TestHeuristicSkipsAmbiguousSlashDates(t *testing.T) {
	proposal := extractHeuristic(t, "Invoice Date: 03/04/2026\nDue Date: 05/04/2026\n")

	if proposal.IssueDate != nil || proposal.DueDate != nil {
		t.Errorf("slash dates must not be guessed, got issue=%v due=%v",
			value(t, proposal.IssueDate), value(t, proposal.DueDate))
	}
}

func TestHeuristicReadsUnambiguousDateFormats(t *testing.T) {
	for _, testCase := range []struct{ name, text, want string }{
		{"iso", "Invoice Date: 2026-05-04", "2026-05-04"},
		{"dotted", "Invoice Date: 04.05.2026", "2026-05-04"},
		{"day first", "Invoice Date: 4 May 2026", "2026-05-04"},
		{"month first", "Invoice Date: May 4, 2026", "2026-05-04"},
		{"abbreviated", "Invoice Date: 4 Sep 2026", "2026-09-04"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			proposal := extractHeuristic(t, testCase.text)
			if got := value(t, proposal.IssueDate); got != testCase.want {
				t.Errorf("issue_date = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestHeuristicEvidenceQuotesRealSourceLines(t *testing.T) {
	proposal := extractHeuristic(t, ordinaryInvoice)

	if len(proposal.Evidence) == 0 {
		t.Fatal("expected evidence for the values that were read")
	}
	// The whole point of adapter-supplied evidence is that the server can prove
	// it. Running the real check is the assertion.
	pages := []PageText{{PageNumber: 1, Text: ordinaryInvoice}}
	if err := ValidateEvidence(proposal, pages, DefaultLimits()); err != nil {
		t.Fatalf("evidence must survive the server check: %v", err)
	}
	for _, evidence := range proposal.Evidence {
		if !strings.Contains(ordinaryInvoice, evidence.Excerpt) {
			t.Errorf("excerpt %q is not a literal source substring", evidence.Excerpt)
		}
	}
}

func TestHeuristicDoesNotReadAPercentageAsAnAmount(t *testing.T) {
	proposal := extractHeuristic(t, "VAT (19%)                 13.49\nTotal due                 84.49\n")

	if got := value(t, proposal.TaxAmount); got != "13.49" {
		t.Errorf("tax_amount = %q, want the amount 13.49 rather than the 19%% rate", got)
	}
}

func TestHeuristicIgnoresARateWithNoAmount(t *testing.T) {
	// A rate on its own must produce no candidate at all; inventing 19.00 here
	// would put a fabricated tax amount in front of the reviewer.
	proposal := extractHeuristic(t, "Invoice No: A-1\nVAT (19 %)\n")

	if proposal.TaxAmount != nil {
		t.Errorf("tax_amount = %q, want unset when only a rate is present", *proposal.TaxAmount)
	}
}

func TestHeuristicDoesNotTreatTaxIDAsTaxAmount(t *testing.T) {
	proposal := extractHeuristic(t, "Tax ID: 555123\nTotal due 40.00\n")

	if proposal.TaxAmount != nil {
		t.Errorf("tax_amount = %q, want unset for a registration number", *proposal.TaxAmount)
	}
	if got := value(t, proposal.Total); got != "40.00" {
		t.Errorf("total = %q, want 40.00", got)
	}
}

func TestHeuristicDoesNotTreatSummaryRowsAsLineItems(t *testing.T) {
	proposal := extractHeuristic(t, ordinaryInvoice)

	for _, item := range proposal.LineItems {
		description := strings.ToLower(value(t, item.Description))
		if strings.Contains(description, "subtotal") || strings.Contains(description, "total") || strings.Contains(description, "vat") {
			t.Errorf("summary row leaked into line items: %q", description)
		}
	}
}

func TestHeuristicSeparatesSubtotalFromTotal(t *testing.T) {
	proposal := extractHeuristic(t, "Subtotal 71.00\nTotal 84.49\n")

	if got := value(t, proposal.Subtotal); got != "71.00" {
		t.Errorf("subtotal = %q, want 71.00", got)
	}
	if got := value(t, proposal.Total); got != "84.49" {
		t.Errorf("total = %q, want 84.49", got)
	}
}

func TestHeuristicStripsGroupingSeparators(t *testing.T) {
	proposal := extractHeuristic(t, "Total due USD 1,234.50\n")

	// money-v1 rejects grouping separators, so the adapter must emit the plain
	// ASCII decimal rather than push the problem into normalization.
	if got := value(t, proposal.Total); got != "1234.50" {
		t.Errorf("total = %q, want 1234.50", got)
	}
}

func TestHeuristicRespectsLimits(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxLineItems = 1
	limits.MaxEvidence = 2

	proposal, err := HeuristicStructuredExtractor{}.Extract(context.Background(), heuristicInput(ordinaryInvoice), limits)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(proposal.LineItems) > limits.MaxLineItems {
		t.Errorf("line items = %d, want at most %d", len(proposal.LineItems), limits.MaxLineItems)
	}
	if len(proposal.Evidence) > limits.MaxEvidence {
		t.Errorf("evidence = %d, want at most %d", len(proposal.Evidence), limits.MaxEvidence)
	}
	if err := limits.ValidateProposal(proposal); err != nil {
		t.Errorf("result must satisfy the limits it was built under: %v", err)
	}
}

func TestHeuristicRejectsInvalidInput(t *testing.T) {
	_, err := HeuristicStructuredExtractor{}.Extract(context.Background(),
		StructuredExtractionInput{DocumentSHA256: "not-a-hash"}, DefaultLimits())
	if err == nil {
		t.Fatal("expected invalid input to be rejected")
	}
}

func TestHeuristicHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := (HeuristicStructuredExtractor{}).Extract(ctx, heuristicInput(ordinaryInvoice), DefaultLimits()); err == nil {
		t.Fatal("expected a cancelled context to be honored")
	}
}

func TestHeuristicTruncatesExcerptOnRuneBoundary(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxEvidenceExcerptBytes = 9

	long := strings.Repeat("é", 40)
	proposal, err := HeuristicStructuredExtractor{}.Extract(context.Background(),
		heuristicInput(long+"\nInvoice No: X-1\n"), limits)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	for _, evidence := range proposal.Evidence {
		if len(evidence.Excerpt) > limits.MaxEvidenceExcerptBytes {
			t.Errorf("excerpt is %d bytes, want at most %d", len(evidence.Excerpt), limits.MaxEvidenceExcerptBytes)
		}
		if !strings.Contains(long+"\nInvoice No: X-1\n", evidence.Excerpt) {
			t.Errorf("excerpt %q stopped being a source substring", evidence.Excerpt)
		}
	}
}
