package invoices

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/reinhlord/invoiceflow/internal/extraction"
)

// Warning is server-generated validation context, not provider confidence.
type Warning struct {
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type NormalizedLineItem struct {
	Description string `json:"description,omitempty"`
	Quantity    string `json:"quantity,omitempty"`
	UnitPrice   *int64 `json:"unit_price_minor,omitempty"`
	TaxAmount   *int64 `json:"tax_amount_minor,omitempty"`
	Total       *int64 `json:"total_minor,omitempty"`
}

type NormalizedProposal struct {
	RoundingPolicyVersion string               `json:"rounding_policy_version"`
	SupplierName          string               `json:"supplier_name,omitempty"`
	SupplierEmail         string               `json:"supplier_email,omitempty"`
	InvoiceNumber         string               `json:"invoice_number,omitempty"`
	IssueDate             string               `json:"issue_date,omitempty"`
	DueDate               string               `json:"due_date,omitempty"`
	Currency              string               `json:"currency,omitempty"`
	Subtotal              *int64               `json:"subtotal_minor,omitempty"`
	TaxAmount             *int64               `json:"tax_amount_minor,omitempty"`
	Total                 *int64               `json:"total_minor,omitempty"`
	LineItems             []NormalizedLineItem `json:"line_items"`
}

// NormalizeProposal makes untrusted candidate values displayable and
// arithmetic-checkable. It never fills missing money with zero or decides any
// workflow state.
func NormalizeProposal(proposal extraction.Proposal) (NormalizedProposal, []Warning) {
	result := NormalizedProposal{RoundingPolicyVersion: RoundingPolicyV1, LineItems: make([]NormalizedLineItem, 0, len(proposal.LineItems))}
	warnings := make([]Warning, 0)
	result.SupplierName = cleanCandidate(proposal.SupplierName)
	result.SupplierEmail = cleanCandidate(proposal.SupplierEmail)
	result.InvoiceNumber = cleanCandidate(proposal.InvoiceNumber)
	result.IssueDate, warnings = normalizeDate("issue_date", proposal.IssueDate, warnings)
	result.DueDate, warnings = normalizeDate("due_date", proposal.DueDate, warnings)
	currency := strings.ToUpper(strings.TrimSpace(cleanCandidate(proposal.Currency)))
	exponent, currencyOK := CurrencyExponent(currency)
	if currency != "" && !currencyOK {
		warnings = append(warnings, warning("unsupported_currency", "currency", "Currency is not supported by the current normalization policy."))
	}
	if currencyOK {
		result.Currency = currency
	}
	result.Subtotal, warnings = normalizeMoney("subtotal", proposal.Subtotal, exponent, currencyOK, warnings)
	result.TaxAmount, warnings = normalizeMoney("tax_amount", proposal.TaxAmount, exponent, currencyOK, warnings)
	result.Total, warnings = normalizeMoney("total", proposal.Total, exponent, currencyOK, warnings)

	var lineSum *int64
	lineSumOverflow := false
	allLineTotals := len(proposal.LineItems) > 0
	for index, item := range proposal.LineItems {
		line, lineWarnings := normalizeLine(index, item, exponent, currencyOK)
		warnings = append(warnings, lineWarnings...)
		result.LineItems = append(result.LineItems, line)
		if line.Total != nil && !lineSumOverflow {
			var added bool
			lineSum, added = addPointers(lineSum, line.Total)
			if !added {
				lineSumOverflow = true
				warnings = append(warnings, warning("line_items_subtotal_overflow", "subtotal", "Line-item totals exceed the supported exact range."))
			}
		} else {
			allLineTotals = false
		}
	}
	if allLineTotals && !lineSumOverflow && lineSum != nil && result.Subtotal != nil && *lineSum != *result.Subtotal {
		warnings = append(warnings, warning("line_items_subtotal_mismatch", "subtotal", "Line-item totals do not equal subtotal."))
	}
	if result.Subtotal != nil && result.TaxAmount != nil && result.Total != nil {
		if sum, ok := checkedAdd(*result.Subtotal, *result.TaxAmount); !ok {
			warnings = append(warnings, warning("subtotal_tax_total_overflow", "total", "Subtotal plus tax exceeds the supported exact range."))
		} else if sum != *result.Total {
			warnings = append(warnings, warning("subtotal_tax_total_mismatch", "total", "Subtotal plus tax does not equal total."))
		}
	}
	warnings = append(warnings, missingFieldWarnings(proposal)...)
	return result, warnings
}

// requiredProposalFields are the values a reviewer needs before an invoice can
// reasonably be approved. Subtotal and tax are deliberately absent: plenty of
// legitimate invoices state only a total, and warning about those would train
// the reviewer to ignore warnings.
var requiredProposalFields = []string{"supplier_name", "invoice_number", "issue_date", "currency", "total"}

// missingFieldWarnings reports required values the extractor did not supply at
// all. A value that was supplied but rejected already produced its own specific
// warning (invalid_date, invalid_money, unsupported_currency), so this checks
// the raw candidate rather than the normalized result and cannot double-report
// the same field.
func missingFieldWarnings(proposal extraction.Proposal) []Warning {
	raw := map[string]*string{
		"supplier_name":  proposal.SupplierName,
		"invoice_number": proposal.InvoiceNumber,
		"issue_date":     proposal.IssueDate,
		"currency":       proposal.Currency,
		"total":          proposal.Total,
	}
	warnings := make([]Warning, 0, len(requiredProposalFields))
	for _, field := range requiredProposalFields {
		if cleanCandidate(raw[field]) == "" {
			warnings = append(warnings, warning("missing_required_field", field, "This value is required for review but was not extracted."))
		}
	}
	return warnings
}

func normalizeLine(index int, item extraction.LineItemProposal, exponent int, currencyOK bool) (NormalizedLineItem, []Warning) {
	field := fmt.Sprintf("line_items.%d", index)
	line := NormalizedLineItem{Description: cleanCandidate(item.Description), Quantity: cleanCandidate(item.Quantity)}
	warnings := []Warning{}
	line.UnitPrice, warnings = normalizeMoney(field+".unit_price", item.UnitPrice, exponent, currencyOK, warnings)
	line.TaxAmount, warnings = normalizeMoney(field+".tax_amount", item.TaxAmount, exponent, currencyOK, warnings)
	line.Total, warnings = normalizeMoney(field+".total", item.Total, exponent, currencyOK, warnings)
	if line.Quantity != "" {
		quantity, err := ParseExactDecimal(line.Quantity)
		if err != nil || decimalPlaces(line.Quantity) > 6 {
			warnings = append(warnings, warning("invalid_quantity", field+".quantity", "Quantity is not a supported exact decimal."))
		} else if line.UnitPrice != nil && line.TaxAmount != nil && line.Total != nil {
			expected, multiplyErr := multiplyMinor(quantity, *line.UnitPrice)
			if multiplyErr != nil {
				warnings = append(warnings, warning("line_arithmetic_overflow", field+".total", "Line arithmetic exceeds supported exact range."))
			} else if new(big.Int).Add(big.NewInt(expected), big.NewInt(*line.TaxAmount)).Cmp(big.NewInt(*line.Total)) != 0 {
				warnings = append(warnings, warning("line_total_mismatch", field+".total", "Quantity, unit price, and tax do not equal the line total."))
			}
		}
	}
	return line, warnings
}

func multiplyMinor(quantity *big.Rat, minor int64) (int64, error) {
	value := new(big.Rat).Mul(quantity, new(big.Rat).SetInt64(minor))
	return roundRatToInt64V1(value)
}
func decimalPlaces(value string) int {
	if dot := strings.IndexByte(value, '.'); dot >= 0 {
		return len(value) - dot - 1
	}
	return 0
}
func cleanCandidate(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
func normalizeDate(field string, value *string, warnings []Warning) (string, []Warning) {
	text := cleanCandidate(value)
	if text == "" {
		return "", warnings
	}
	date, err := time.Parse("2006-01-02", text)
	if err != nil || date.Format("2006-01-02") != text {
		return "", append(warnings, warning("invalid_date", field, "Date must use ISO YYYY-MM-DD."))
	}
	return date.UTC().Format("2006-01-02"), warnings
}
func normalizeMoney(field string, value *string, exponent int, currencyOK bool, warnings []Warning) (*int64, []Warning) {
	text := cleanCandidate(value)
	if text == "" {
		return nil, warnings
	}
	if !currencyOK {
		return nil, append(warnings, warning("missing_or_invalid_currency", field, "A supported explicit currency is required for money."))
	}
	minor, err := DecimalToMinorV1(text, exponent)
	if err != nil {
		return nil, append(warnings, warning("invalid_money", field, "Money must be an exact decimal."))
	}
	return &minor, warnings
}
func warning(code, field, message string) Warning {
	return Warning{Code: code, Field: field, Message: message}
}
func addPointers(left, right *int64) (*int64, bool) {
	if left == nil {
		copy := *right
		return &copy, true
	}
	sum, ok := checkedAdd(*left, *right)
	if !ok {
		return nil, false
	}
	return &sum, true
}

func checkedAdd(left, right int64) (int64, bool) {
	if right > 0 && left > math.MaxInt64-right || right < 0 && left < math.MinInt64-right {
		return 0, false
	}
	return left + right, true
}
