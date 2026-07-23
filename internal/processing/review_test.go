package processing

import (
	"strings"
	"testing"
)

func TestDecodeHumanReviewInputRejectsAuthorityAndNormalizesCandidates(t *testing.T) {
	input, err := DecodeHumanReviewInput([]byte(`{"supplier_name":"  Fictional Vendor ","currency":" usd ","total":"24.005","line_items":[{"description":"Service","quantity":"1","unit_price":"24.005","tax_amount":"","total":"24.00"}]}`))
	if err != nil {
		t.Fatalf("DecodeHumanReviewInput() error = %v", err)
	}
	proposal := input.proposal()
	if proposal.Currency == nil || *proposal.Currency != " usd " || len(proposal.LineItems) != 1 {
		t.Fatalf("proposal = %+v", proposal)
	}
	if _, err := DecodeHumanReviewInput([]byte(`{"currency":"USD","status":"approved"}`)); err != ErrInvalidHumanReviewEdit {
		t.Fatalf("unknown authority field error = %v, want ErrInvalidHumanReviewEdit", err)
	}
	if _, err := DecodeHumanReviewInput([]byte(`{"currency":1}`)); err != ErrInvalidHumanReviewEdit {
		t.Fatalf("numeric candidate error = %v, want ErrInvalidHumanReviewEdit", err)
	}
}

func TestEditableFromNormalizedUsesExactMoneyStrings(t *testing.T) {
	raw := []byte(`{"rounding_policy_version":"money-v1","currency":"USD","total_minor":2400,"line_items":[{"quantity":"2","unit_price_minor":1000,"total_minor":2000}]}`)
	editable, err := editableFromNormalized(raw)
	if err != nil {
		t.Fatal(err)
	}
	if editable.Total != "24.00" || editable.LineItems[0].UnitPrice != "10.00" {
		t.Fatalf("editable = %+v", editable)
	}
	if _, err := editableFromNormalized([]byte(`not JSON`)); err == nil || !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("bad JSON error = %v", err)
	}
}
