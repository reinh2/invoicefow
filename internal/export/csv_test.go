package export

import (
	"bytes"
	"encoding/csv"

	"testing"

	"github.com/reinhlord/invoiceflow/internal/invoices"
)

func TestGenerateCSV(t *testing.T) {
	subtotal := int64(2000)
	tax := int64(400)
	total := int64(2400)
	price := int64(1000)
	lineTax := int64(0)
	lineTotal := int64(2000)

	normalized := invoices.NormalizedProposal{
		RoundingPolicyVersion: "money-v1",
		SupplierName:          "Fictional Vendor, Inc. \"Global\"",
		SupplierEmail:         "vendor@example.test",
		InvoiceNumber:         "INV-001",
		IssueDate:             "2026-07-20",
		DueDate:               "2026-08-20",
		Currency:              "USD",
		Subtotal:              &subtotal,
		TaxAmount:             &tax,
		Total:                 &total,
		LineItems: []invoices.NormalizedLineItem{
			{
				Description: "Widget, Standard\nLine 2 Description with Unicode €",
				Quantity:    "2",
				UnitPrice:   &price,
				TaxAmount:   &lineTax,
				Total:       &lineTotal,
			},
		},
	}

	csvBytes, err := GenerateCSV(normalized)
	if err != nil {
		t.Fatalf("GenerateCSV failed: %v", err)
	}

	if !bytes.Contains(csvBytes, []byte("\r\n")) {
		t.Errorf("expected RFC 4180 CRLF line endings")
	}

	reader := csv.NewReader(bytes.NewReader(csvBytes))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse generated CSV: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records (header + line item), got %d", len(records))
	}

	if records[0][0] != "supplier_name" || records[0][8] != "total" {
		t.Fatalf("unexpected header columns: %v", records[0])
	}

	if records[1][0] != "Fictional Vendor, Inc. \"Global\"" {
		t.Errorf("unexpected supplier name parsing: %s", records[1][0])
	}

	if records[1][8] != "24.00" {
		t.Errorf("expected total 24.00, got %s", records[1][8])
	}
}

func TestGenerateCSVNoLineItems(t *testing.T) {
	total := int64(100)
	normalized := invoices.NormalizedProposal{
		SupplierName: "Solo Supplier",
		Currency:     "USD",
		Total:        &total,
		LineItems:    []invoices.NormalizedLineItem{},
	}

	csvBytes, err := GenerateCSV(normalized)
	if err != nil {
		t.Fatalf("GenerateCSV failed: %v", err)
	}

	reader := csv.NewReader(bytes.NewReader(csvBytes))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse generated CSV: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records (header + empty line item row), got %d", len(records))
	}
	if records[1][0] != "Solo Supplier" {
		t.Errorf("unexpected supplier: %s", records[1][0])
	}
	if records[1][9] != "" {
		t.Errorf("expected empty line item description, got %s", records[1][9])
	}
}

func TestGenerateCSVGoldenPublicContract(t *testing.T) {
	negative := int64(-123)
	lineTotal := int64(-123)
	normalized := invoices.NormalizedProposal{
		SupplierName:  "日本商事\n\"quoted\"",
		SupplierEmail: "billing@example.test",
		InvoiceNumber: "JPY-001",
		IssueDate:     "2026-07-24",
		Currency:      "JPY",
		Total:         &negative,
		LineItems:     []invoices.NormalizedLineItem{{Description: "サービス, 日本語", Quantity: "1", Total: &lineTotal}},
	}
	want := "supplier_name,supplier_email,invoice_number,issue_date,due_date,currency,subtotal,tax_amount,total,line_item_description,line_item_quantity,line_item_unit_price,line_item_tax_amount,line_item_total\r\n\"日本商事\r\n\"\"quoted\"\"\",billing@example.test,JPY-001,2026-07-24,,JPY,,,-123,\"サービス, 日本語\",1,,,-123\r\n"
	got, err := GenerateCSV(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("golden CSV mismatch\ngot:  %q\nwant: %q", got, want)
	}
	again, err := GenerateCSV(normalized)
	if err != nil || !bytes.Equal(got, again) {
		t.Fatalf("CSV is not deterministic: err=%v", err)
	}
}
