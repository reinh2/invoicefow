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
