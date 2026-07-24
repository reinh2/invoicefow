package extraction

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testAPIKey = "sk-test-secret-key-value"

func chatResponse(content string) string {
	body, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": content}},
		},
		"usage": map[string]any{"total_tokens": 42},
	})
	return string(body)
}

func newTestExtractor(t *testing.T, handler http.HandlerFunc) (*OpenAIStructuredExtractor, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	extractor, err := NewOpenAIStructuredExtractor(OpenAIOptions{
		APIKey:  testAPIKey,
		Model:   "gpt-4o-mini",
		BaseURL: server.URL,
		Client:  server.Client(),
	})
	if err != nil {
		t.Fatalf("NewOpenAIStructuredExtractor() error = %v", err)
	}
	return extractor, server
}

func testInput() StructuredExtractionInput {
	return StructuredExtractionInput{
		DocumentSHA256: testHash,
		ReferenceText:  []PageText{{PageNumber: 1, Text: "Acme Ltd\nInvoice INV-9\nTotal 24.00"}},
	}
}

func TestNewOpenAIStructuredExtractorRequiresAPIKey(t *testing.T) {
	t.Parallel()
	if _, err := NewOpenAIStructuredExtractor(OpenAIOptions{APIKey: "  "}); !errors.Is(err, ErrOpenAIConfiguration) {
		t.Fatalf("empty key error = %v, want ErrOpenAIConfiguration", err)
	}
}

func TestNewOpenAIStructuredExtractorRejectsNonHTTPBaseURL(t *testing.T) {
	t.Parallel()
	if _, err := NewOpenAIStructuredExtractor(OpenAIOptions{APIKey: testAPIKey, BaseURL: "ftp://example"}); !errors.Is(err, ErrOpenAIConfiguration) {
		t.Fatalf("bad base url error = %v, want ErrOpenAIConfiguration", err)
	}
}

func TestOpenAIExtractDecodesStrictProposal(t *testing.T) {
	t.Parallel()
	var captured struct {
		auth string
		body []byte
	}
	extractor, _ := newTestExtractor(t, func(w http.ResponseWriter, r *http.Request) {
		captured.auth = r.Header.Get("Authorization")
		captured.body, _ = io.ReadAll(r.Body)
		content := `{"supplier_name":"Acme Ltd","supplier_email":null,"invoice_number":"INV-9","issue_date":null,"due_date":null,"currency":"USD","subtotal":"20.00","tax_amount":"4.00","total":"24.00","line_items":[{"description":"Widget","quantity":"2","unit_price":"10.00","tax_amount":null,"total":"20.00"}]}`
		_, _ = io.WriteString(w, chatResponse(content))
	})

	proposal, err := extractor.Extract(context.Background(), testInput(), DefaultLimits())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if proposal.SupplierName == nil || *proposal.SupplierName != "Acme Ltd" {
		t.Fatalf("SupplierName = %v, want Acme Ltd", proposal.SupplierName)
	}
	if proposal.Total == nil || *proposal.Total != "24.00" {
		t.Fatalf("Total = %v, want 24.00", proposal.Total)
	}
	if len(proposal.LineItems) != 1 || proposal.LineItems[0].Description == nil || *proposal.LineItems[0].Description != "Widget" {
		t.Fatalf("LineItems = %#v, want one Widget line", proposal.LineItems)
	}

	// The request must authenticate with the key and pin a strict json_schema.
	if captured.auth != "Bearer "+testAPIKey {
		t.Fatalf("Authorization header = %q, want bearer key", captured.auth)
	}
	var request map[string]any
	if err := json.Unmarshal(captured.body, &request); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if request["model"] != "gpt-4o-mini" {
		t.Fatalf("model = %v, want gpt-4o-mini", request["model"])
	}
	format, _ := request["response_format"].(map[string]any)
	schema, _ := format["json_schema"].(map[string]any)
	if schema["strict"] != true {
		t.Fatalf("response_format.json_schema.strict = %v, want true", schema["strict"])
	}
}

func TestOpenAIExtractDropsModelSuppliedEvidence(t *testing.T) {
	t.Parallel()
	// Even if a provider ignores the schema and returns evidence, the adapter
	// must never assert a source excerpt it cannot prove.
	extractor, _ := newTestExtractor(t, func(w http.ResponseWriter, r *http.Request) {
		content := `{"supplier_name":"Acme Ltd","supplier_email":null,"invoice_number":null,"issue_date":null,"due_date":null,"currency":null,"subtotal":null,"tax_amount":null,"total":null,"line_items":[],"evidence":[{"field":"supplier_name","page_number":1,"excerpt":"fabricated"}]}`
		_, _ = io.WriteString(w, chatResponse(content))
	})
	proposal, err := extractor.Extract(context.Background(), testInput(), DefaultLimits())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if proposal.Evidence != nil {
		t.Fatalf("Evidence = %#v, want nil", proposal.Evidence)
	}
}

func TestOpenAIExtractRejectsRefusal(t *testing.T) {
	t.Parallel()
	extractor, _ := newTestExtractor(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "", "refusal": "I cannot help with that."}},
			},
		})
		_, _ = w.Write(body)
	})
	_, err := extractor.Extract(context.Background(), testInput(), DefaultLimits())
	if !errors.Is(err, ErrOpenAIRequest) {
		t.Fatalf("refusal error = %v, want ErrOpenAIRequest", err)
	}
}

func TestOpenAIExtractRejectsNon200(t *testing.T) {
	t.Parallel()
	extractor, _ := newTestExtractor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Incorrect API key sk-should-not-leak"}}`)
	})
	_, err := extractor.Extract(context.Background(), testInput(), DefaultLimits())
	if !errors.Is(err, ErrOpenAIRequest) {
		t.Fatalf("status error = %v, want ErrOpenAIRequest", err)
	}
	// The error must never carry the API key or the provider body.
	if strings.Contains(err.Error(), testAPIKey) || strings.Contains(err.Error(), "sk-should-not-leak") {
		t.Fatalf("error leaked secret material: %q", err.Error())
	}
}

func TestOpenAIExtractRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	extractor, _ := newTestExtractor(t, func(w http.ResponseWriter, r *http.Request) {
		content := `{"supplier_name":"Acme Ltd","confidence":0.9}`
		_, _ = io.WriteString(w, chatResponse(content))
	})
	_, err := extractor.Extract(context.Background(), testInput(), DefaultLimits())
	if !errors.Is(err, ErrInvalidProposalSchema) {
		t.Fatalf("unknown-field error = %v, want ErrInvalidProposalSchema", err)
	}
}

func TestOpenAIExtractBoundsOversizedResponse(t *testing.T) {
	t.Parallel()
	extractor, _ := newTestExtractor(t, func(w http.ResponseWriter, r *http.Request) {
		huge := strings.Repeat("a", 200<<10)
		_, _ = io.WriteString(w, chatResponse(`{"supplier_name":"`+huge+`"}`))
	})
	_, err := extractor.Extract(context.Background(), testInput(), DefaultLimits())
	if !errors.Is(err, ErrOpenAIRequest) {
		t.Fatalf("oversized error = %v, want ErrOpenAIRequest", err)
	}
}

func TestOpenAIExtractValidatesInputBeforeCall(t *testing.T) {
	t.Parallel()
	called := false
	extractor, _ := newTestExtractor(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = io.WriteString(w, chatResponse(`{}`))
	})
	// An invalid SHA-256 must be rejected without any network call.
	_, err := extractor.Extract(context.Background(), StructuredExtractionInput{DocumentSHA256: "short"}, DefaultLimits())
	if err == nil {
		t.Fatal("Extract() error = nil, want validation error")
	}
	if called {
		t.Fatal("adapter called the provider despite invalid input")
	}
}
