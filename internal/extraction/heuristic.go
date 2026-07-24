package extraction

import (
	"context"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// HeuristicDiagnosticCode marks a proposal produced by the offline heuristic
// reader rather than by a fixture or a model. The worker allowlists it so the
// review UI can show the human where the candidates came from.
const HeuristicDiagnosticCode = "heuristic_extraction"

// maxHeuristicLines bounds the scan independently of the reference-text byte
// limit, so a pathological page of very short lines cannot turn a bounded input
// into an unbounded number of regexp passes.
const maxHeuristicLines = 5000

// HeuristicStructuredExtractor is a deterministic, offline fallback reader. It
// exists so the no-key demo shows real work on a document that is not one of
// the committed fixtures, instead of an empty form.
//
// It is deliberately conservative and is not an accuracy claim:
//
//   - It never invents a value. A pattern that does not match leaves the
//     candidate nil, so the server's normalizer reports it rather than a zero
//     or a guess appearing in the review form.
//   - It reads only unambiguous date formats (ISO YYYY-MM-DD, DD.MM.YYYY, and
//     English month names). Slash dates are skipped on purpose: 03/04/2026 is
//     locale-ambiguous, and ADR-007 forbids locale guessing.
//   - Its evidence is truthful. Every entry quotes the exact source line the
//     candidate was read from, so ValidateEvidence's substring check passes on
//     real data rather than on a synthesized excerpt.
//
// Like every adapter behind StructuredExtractor it makes no network call, holds
// no state, and has no workflow authority: the result is re-validated and
// normalized server-side exactly like a model response.
type HeuristicStructuredExtractor struct{}

var (
	heuristicInvoiceNumber = regexp.MustCompile(`(?i)\binvoice\s*(?:no\.?|number|num\.?|#)?\s*[:#]?\s*([A-Za-z0-9][A-Za-z0-9\-/_]{2,31})\b`)
	heuristicEmail         = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,24}`)
	heuristicISODate       = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)
	heuristicDottedDate    = regexp.MustCompile(`\b(\d{1,2})\.(\d{1,2})\.(\d{4})\b`)
	heuristicMonthFirst    = regexp.MustCompile(`(?i)\b([A-Za-z]{3,9})\.?\s+(\d{1,2})(?:st|nd|rd|th)?,?\s+(\d{4})\b`)
	heuristicDayFirst      = regexp.MustCompile(`(?i)\b(\d{1,2})(?:st|nd|rd|th)?\s+([A-Za-z]{3,9})\.?,?\s+(\d{4})\b`)
	heuristicAmount        = regexp.MustCompile(`-?\d[\d,]*(?:\.\d{1,2})?`)
	heuristicCurrencyCode  = regexp.MustCompile(`(?i)\b(USD|EUR|GBP|RUB|JPY)\b`)

	heuristicSubtotalLabel = regexp.MustCompile(`(?i)\b(sub-?\s?total|net\s+amount|amount\s+before\s+tax)\b`)
	heuristicTaxLabel      = regexp.MustCompile(`(?i)\b(tax|vat|gst)\b`)
	heuristicTaxIDLabel    = regexp.MustCompile(`(?i)\btax\s*(id|identification|number|no\.?|reg)\b`)
	heuristicTotalLabel    = regexp.MustCompile(`(?i)\b(grand\s+total|total\s+due|amount\s+due|balance\s+due|total)\b`)
	heuristicDueDateLabel  = regexp.MustCompile(`(?i)\b(due\s+date|payment\s+due|due\s+by|payable\s+by)\b`)
	heuristicIssueLabel    = regexp.MustCompile(`(?i)\b(invoice\s+date|issue\s+date|date\s+of\s+issue|dated|date)\b`)

	// A line item is accepted only in the unambiguous four-column shape
	// "description … quantity unit-price line-total", where both prices carry
	// explicit minor units. Anything looser produces junk rows more often than
	// useful ones, so it is rejected instead.
	heuristicLineItem = regexp.MustCompile(`^(.*?\S)\s+(\d+(?:\.\d{1,6})?)\s+(-?\d[\d,]*\.\d{2})\s+(-?\d[\d,]*\.\d{2})$`)
)

var heuristicMonths = map[string]string{
	"january": "01", "jan": "01",
	"february": "02", "feb": "02",
	"march": "03", "mar": "03",
	"april": "04", "apr": "04",
	"may":  "05",
	"june": "06", "jun": "06",
	"july": "07", "jul": "07",
	"august": "08", "aug": "08",
	"september": "09", "sep": "09", "sept": "09",
	"october": "10", "oct": "10",
	"november": "11", "nov": "11",
	"december": "12", "dec": "12",
}

var heuristicCurrencySymbols = map[string]string{
	"$": "USD", "€": "EUR", "£": "GBP", "₽": "RUB", "¥": "JPY",
}

// sourceLine is one trimmed line of bounded reference text together with the
// one-based page it came from, so evidence can quote it truthfully.
type sourceLine struct {
	page int
	text string
}

// Extract reads candidates from bounded reference text. It returns the same
// empty-but-diagnosed proposal shape as any other adapter when it recognizes
// nothing, and never returns a value it did not literally read.
func (HeuristicStructuredExtractor) Extract(ctx context.Context, input StructuredExtractionInput, limits Limits) (Proposal, error) {
	if err := ctx.Err(); err != nil {
		return Proposal{}, err
	}
	if err := limits.ValidateStructuredInput(input); err != nil {
		return Proposal{}, err
	}

	lines := heuristicLines(input.ReferenceText)
	builder := &heuristicProposal{limits: limits}

	builder.readSupplierEmail(lines)
	builder.readInvoiceNumber(lines)
	builder.readDates(lines)
	builder.readCurrency(lines)
	builder.readAmounts(lines)
	builder.readLineItems(lines)
	builder.readSupplierName(lines)

	proposal := builder.proposal
	proposal.Diagnostics = []Diagnostic{{
		Code:    HeuristicDiagnosticCode,
		Message: "Values were read by the offline heuristic reader. Verify every field against the original.",
	}}
	if err := limits.ValidateProposal(proposal); err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

func heuristicLines(pages []PageText) []sourceLine {
	lines := make([]sourceLine, 0, 64)
	for _, page := range pages {
		for _, raw := range strings.Split(page.Text, "\n") {
			text := strings.TrimSpace(raw)
			if text == "" {
				continue
			}
			lines = append(lines, sourceLine{page: page.PageNumber, text: text})
			if len(lines) >= maxHeuristicLines {
				return lines
			}
		}
	}
	return lines
}

// heuristicProposal accumulates candidates and the evidence proving each one.
type heuristicProposal struct {
	limits   Limits
	proposal Proposal
}

// set records one candidate together with the source line it was read from.
// Evidence is added only while it stays inside the configured bound, so a long
// document degrades to fewer evidence entries rather than to a rejected result.
func (h *heuristicProposal) set(target **string, field string, value string, line sourceLine) {
	if *target != nil || value == "" {
		return
	}
	candidate := value
	*target = &candidate
	if len(h.proposal.Evidence) >= h.limits.MaxEvidence {
		return
	}
	excerpt := truncateRunes(line.text, h.limits.MaxEvidenceExcerptBytes)
	if excerpt == "" {
		return
	}
	h.proposal.Evidence = append(h.proposal.Evidence, Evidence{Field: field, PageNumber: line.page, Excerpt: excerpt})
}

func (h *heuristicProposal) readSupplierEmail(lines []sourceLine) {
	for _, line := range lines {
		if match := heuristicEmail.FindString(line.text); match != "" {
			h.set(&h.proposal.SupplierEmail, "supplier_email", match, line)
			return
		}
	}
}

func (h *heuristicProposal) readInvoiceNumber(lines []sourceLine) {
	for _, line := range lines {
		match := heuristicInvoiceNumber.FindStringSubmatch(line.text)
		if match == nil {
			continue
		}
		candidate := strings.Trim(match[1], "-/_")
		// "Invoice Date" and friends match the label pattern; a candidate that is
		// a bare word with no digit is a label, not an identifier.
		if candidate == "" || !strings.ContainsFunc(candidate, unicode.IsDigit) {
			continue
		}
		h.set(&h.proposal.InvoiceNumber, "invoice_number", candidate, line)
		return
	}
}

func (h *heuristicProposal) readDates(lines []sourceLine) {
	for _, line := range lines {
		date := heuristicDate(line.text)
		if date == "" {
			continue
		}
		// Due-date labels are checked first because "due date" also contains the
		// generic "date" token that marks an issue date.
		switch {
		case heuristicDueDateLabel.MatchString(line.text):
			h.set(&h.proposal.DueDate, "due_date", date, line)
		case heuristicIssueLabel.MatchString(line.text):
			h.set(&h.proposal.IssueDate, "issue_date", date, line)
		}
	}
}

// heuristicDate returns an ISO date for the unambiguous formats only. Slash
// dates are intentionally not read: they cannot be resolved without guessing a
// locale, and a wrong date is worse than a reported missing one.
func heuristicDate(text string) string {
	if match := heuristicISODate.FindStringSubmatch(text); match != nil {
		return validISODate(match[1], match[2], match[3])
	}
	if match := heuristicDottedDate.FindStringSubmatch(text); match != nil {
		return validISODate(match[3], pad2(match[2]), pad2(match[1]))
	}
	if match := heuristicDayFirst.FindStringSubmatch(text); match != nil {
		if month, ok := heuristicMonths[strings.ToLower(match[2])]; ok {
			return validISODate(match[3], month, pad2(match[1]))
		}
	}
	if match := heuristicMonthFirst.FindStringSubmatch(text); match != nil {
		if month, ok := heuristicMonths[strings.ToLower(match[1])]; ok {
			return validISODate(match[3], month, pad2(match[2]))
		}
	}
	return ""
}

func (h *heuristicProposal) readCurrency(lines []sourceLine) {
	for _, line := range lines {
		if match := heuristicCurrencyCode.FindString(line.text); match != "" {
			h.set(&h.proposal.Currency, "currency", strings.ToUpper(match), line)
			return
		}
	}
	for _, line := range lines {
		for symbol, code := range heuristicCurrencySymbols {
			if strings.Contains(line.text, symbol) {
				h.set(&h.proposal.Currency, "currency", code, line)
				return
			}
		}
	}
}

func (h *heuristicProposal) readAmounts(lines []sourceLine) {
	for _, line := range lines {
		amount := lastAmount(line.text)
		if amount == "" {
			continue
		}
		// Subtotal is matched before total because "subtotal" contains "total",
		// and a tax-registration line is never a tax amount.
		switch {
		case heuristicSubtotalLabel.MatchString(line.text):
			h.set(&h.proposal.Subtotal, "subtotal", amount, line)
		case heuristicTaxLabel.MatchString(line.text) && !heuristicTaxIDLabel.MatchString(line.text):
			h.set(&h.proposal.TaxAmount, "tax_amount", amount, line)
		case heuristicTotalLabel.MatchString(line.text):
			h.set(&h.proposal.Total, "total", amount, line)
		}
	}
}

func (h *heuristicProposal) readLineItems(lines []sourceLine) {
	for _, line := range lines {
		if len(h.proposal.LineItems) >= h.limits.MaxLineItems {
			return
		}
		match := heuristicLineItem.FindStringSubmatch(line.text)
		if match == nil {
			continue
		}
		description := strings.TrimSpace(match[1])
		// A summary row ("Subtotal 80.00") is not a line item, and a description
		// with no letter is table noise rather than a product.
		if description == "" || !strings.ContainsFunc(description, unicode.IsLetter) {
			continue
		}
		if heuristicSubtotalLabel.MatchString(description) || heuristicTotalLabel.MatchString(description) || heuristicTaxLabel.MatchString(description) {
			continue
		}
		item := LineItemProposal{
			Description: stringPointer(description),
			Quantity:    stringPointer(match[2]),
			UnitPrice:   stringPointer(stripGrouping(match[3])),
			Total:       stringPointer(stripGrouping(match[4])),
			// Per-line tax is left unknown rather than assumed zero, so the
			// normalizer's line arithmetic check stays silent instead of raising a
			// mismatch this reader cannot substantiate.
		}
		h.proposal.LineItems = append(h.proposal.LineItems, item)
		if len(h.proposal.Evidence) < h.limits.MaxEvidence {
			if excerpt := truncateRunes(line.text, h.limits.MaxEvidenceExcerptBytes); excerpt != "" {
				h.proposal.Evidence = append(h.proposal.Evidence, Evidence{Field: "line_items", PageNumber: line.page, Excerpt: excerpt})
			}
		}
	}
}

// readSupplierName runs last so it can skip lines already recognized as
// something else. It takes the first plausible heading on the first page, which
// is where a supplier letterhead sits on an ordinary invoice.
//
// "First heading on page one" is by far the weakest signal here — on a document
// that is not an invoice at all it would match ordinary prose. So it requires
// corroboration: unless some other candidate was already read, this reader
// proposes nothing rather than promoting a random first line to a supplier.
func (h *heuristicProposal) readSupplierName(lines []sourceLine) {
	if !h.proposal.HasCandidates() {
		return
	}
	for _, line := range lines {
		if line.page != 1 {
			return
		}
		text := line.text
		if utf8.RuneCountInString(text) < 2 || utf8.RuneCountInString(text) > 80 {
			continue
		}
		if !strings.ContainsFunc(text, unicode.IsLetter) {
			continue
		}
		if heuristicEmail.MatchString(text) || heuristicDate(text) != "" {
			continue
		}
		if heuristicInvoiceNumber.MatchString(text) || heuristicSubtotalLabel.MatchString(text) ||
			heuristicTotalLabel.MatchString(text) || heuristicTaxLabel.MatchString(text) ||
			heuristicDueDateLabel.MatchString(text) || heuristicIssueLabel.MatchString(text) {
			continue
		}
		h.set(&h.proposal.SupplierName, "supplier_name", text, line)
		return
	}
}

// lastAmount returns the rightmost amount on a line, which is where an invoice
// summary row puts its value. Grouping separators are removed so the result is
// the plain ASCII decimal the money-v1 policy accepts.
//
// A rate is not an amount: on "VAT (19%) 13.49" the 19 must never become the
// tax value. Percentages are therefore skipped, and a row that offers nothing
// but a rate yields no candidate at all.
func lastAmount(text string) string {
	matches := heuristicAmount.FindAllStringIndex(text, -1)
	for index := len(matches) - 1; index >= 0; index-- {
		start, end := matches[index][0], matches[index][1]
		if isPercentage(text, end) {
			continue
		}
		candidate := stripGrouping(text[start:end])
		if candidate != "" && candidate != "-" {
			return candidate
		}
	}
	return ""
}

// isPercentage reports whether the number ending at end is immediately followed
// by a percent sign, allowing for the spacing real invoices use ("19 %").
func isPercentage(text string, end int) bool {
	for index := end; index < len(text); index++ {
		switch text[index] {
		case ' ', '\t':
			continue
		case '%':
			return true
		default:
			return false
		}
	}
	return false
}

func stripGrouping(value string) string { return strings.ReplaceAll(value, ",", "") }

func validISODate(year, month, day string) string {
	if len(year) != 4 || len(month) != 2 || len(day) != 2 {
		return ""
	}
	if month < "01" || month > "12" || day < "01" || day > "31" {
		return ""
	}
	return year + "-" + month + "-" + day
}

func pad2(value string) string {
	if len(value) == 1 {
		return "0" + value
	}
	return value
}

func stringPointer(value string) *string { return &value }

// truncateRunes bounds an excerpt in bytes without splitting a rune, so the
// result stays valid UTF-8 and remains a literal substring of the source page.
func truncateRunes(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
}
