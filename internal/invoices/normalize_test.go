package invoices

import (
	"math"
	"testing"

	"github.com/reinhlord/invoiceflow/internal/extraction"
)

func stringPointer(value string) *string { return &value }

func TestDecimalToMinorV1UsesBankersRounding(t *testing.T) {
	cases := []struct {
		value string
		want  int64
	}{{"1.005", 100}, {"1.015", 102}, {"-1.005", -100}, {"-1.015", -102}, {"12.34", 1234}}
	for _, test := range cases {
		got, err := DecimalToMinorV1(test.value, 2)
		if err != nil || got != test.want {
			t.Fatalf("DecimalToMinorV1(%q) = %d, %v; want %d", test.value, got, err, test.want)
		}
	}
	if _, err := DecimalToMinorV1("1,200.00", 2); err == nil {
		t.Fatal("grouped decimal was accepted")
	}
}

func TestNormalizeProposalWarnsInsteadOfWrappingAggregateMoney(t *testing.T) {
	maximum := "92233720368547758.07"
	oneCent := "0.01"
	proposal := extraction.Proposal{
		Currency:  stringPointer("USD"),
		Subtotal:  stringPointer(maximum),
		TaxAmount: stringPointer(oneCent),
		Total:     stringPointer(maximum),
		LineItems: []extraction.LineItemProposal{
			{Total: stringPointer(maximum)},
			{Total: stringPointer(oneCent)},
		},
	}
	normalized, warnings := NormalizeProposal(proposal)
	if normalized.Subtotal == nil || *normalized.Subtotal != math.MaxInt64 {
		t.Fatalf("subtotal = %#v, want MaxInt64", normalized.Subtotal)
	}
	codes := map[string]bool{}
	for _, item := range warnings {
		codes[item.Code] = true
	}
	for _, code := range []string{"line_items_subtotal_overflow", "subtotal_tax_total_overflow"} {
		if !codes[code] {
			t.Fatalf("warning %q missing: %#v", code, warnings)
		}
	}
}

func TestNormalizeProposalRetainsPartialValuesAndGeneratesArithmeticWarnings(t *testing.T) {
	proposal := extraction.Proposal{Currency: stringPointer("usd"), Subtotal: stringPointer("20.00"), TaxAmount: stringPointer("4.00"), Total: stringPointer("25.00"), IssueDate: stringPointer("07/01/2026"), LineItems: []extraction.LineItemProposal{{Quantity: stringPointer("2"), UnitPrice: stringPointer("10.00"), TaxAmount: stringPointer("0.00"), Total: stringPointer("19.99")}}}
	normalized, warnings := NormalizeProposal(proposal)
	if normalized.RoundingPolicyVersion != RoundingPolicyV1 || normalized.Currency != "USD" || normalized.Subtotal == nil || *normalized.Subtotal != 2000 || normalized.IssueDate != "" {
		t.Fatalf("normalized = %#v", normalized)
	}
	codes := map[string]bool{}
	for _, item := range warnings {
		codes[item.Code] = true
	}
	for _, code := range []string{"invalid_date", "line_total_mismatch", "subtotal_tax_total_mismatch"} {
		if !codes[code] {
			t.Fatalf("warning %q missing: %#v", code, warnings)
		}
	}
}

func TestMissingRequiredFieldsAreReported(t *testing.T) {
	// An extractor that recognized nothing must not pass silently: before this
	// warning existed, an entirely empty proposal reached the reviewer with no
	// warnings at all.
	_, warnings := NormalizeProposal(extraction.Proposal{})

	missing := map[string]bool{}
	for _, w := range warnings {
		if w.Code == "missing_required_field" {
			missing[w.Field] = true
		}
	}
	for _, field := range []string{"supplier_name", "invoice_number", "issue_date", "currency", "total"} {
		if !missing[field] {
			t.Errorf("expected a missing_required_field warning for %s", field)
		}
	}
}

func TestCompleteProposalHasNoMissingFieldWarning(t *testing.T) {
	_, warnings := NormalizeProposal(extraction.Proposal{
		SupplierName: stringPointer("Acme"), InvoiceNumber: stringPointer("A-1"), IssueDate: stringPointer("2026-05-04"),
		Currency: stringPointer("USD"), Subtotal: stringPointer("10.00"), TaxAmount: stringPointer("2.00"), Total: stringPointer("12.00"),
	})

	for _, w := range warnings {
		if w.Code == "missing_required_field" {
			t.Errorf("unexpected missing_required_field for %s on a complete proposal", w.Field)
		}
	}
}

func TestInvalidValueIsNotAlsoReportedAsMissing(t *testing.T) {
	_, warnings := NormalizeProposal(extraction.Proposal{
		SupplierName: stringPointer("Acme"), InvoiceNumber: stringPointer("A-1"),
		IssueDate: stringPointer("04.05.2026"), Currency: stringPointer("USD"), Total: stringPointer("12.00"),
	})

	// The date was supplied and rejected, so it must produce invalid_date only —
	// reporting it as missing as well would double-count one problem.
	var invalid, missing int
	for _, w := range warnings {
		if w.Field != "issue_date" {
			continue
		}
		switch w.Code {
		case "invalid_date":
			invalid++
		case "missing_required_field":
			missing++
		}
	}
	if invalid != 1 || missing != 0 {
		t.Errorf("issue_date warnings: invalid_date=%d missing_required_field=%d, want 1 and 0", invalid, missing)
	}
}
