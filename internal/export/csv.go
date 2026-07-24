package export

import (
	"bytes"
	"encoding/csv"

	"github.com/reinhlord/invoiceflow/internal/invoices"
)

const FormatVersionV1 = "csv-v1"

func GenerateCSV(normalized invoices.NormalizedProposal) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	writer.UseCRLF = true

	header := []string{
		"supplier_name", "supplier_email", "invoice_number", "issue_date", "due_date",
		"currency", "subtotal", "tax_amount", "total",
		"line_item_description", "line_item_quantity", "line_item_unit_price", "line_item_tax_amount", "line_item_total",
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}

	exponent, validCurrency := invoices.CurrencyExponent(normalized.Currency)
	formatMoney := func(value *int64) string {
		if value == nil || !validCurrency {
			return ""
		}
		return invoices.MinorToDecimalV1(*value, exponent)
	}

	meta := []string{
		normalized.SupplierName,
		normalized.SupplierEmail,
		normalized.InvoiceNumber,
		normalized.IssueDate,
		normalized.DueDate,
		normalized.Currency,
		formatMoney(normalized.Subtotal),
		formatMoney(normalized.TaxAmount),
		formatMoney(normalized.Total),
	}

	if len(normalized.LineItems) == 0 {
		row := append(meta, "", "", "", "", "")
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	} else {
		for _, line := range normalized.LineItems {
			row := append(meta,
				line.Description,
				line.Quantity,
				formatMoney(line.UnitPrice),
				formatMoney(line.TaxAmount),
				formatMoney(line.Total),
			)
			if err := writer.Write(row); err != nil {
				return nil, err
			}
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
