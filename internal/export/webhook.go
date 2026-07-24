package export

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/reinhlord/invoiceflow/internal/invoices"
)

var (
	ErrInvalidDestination = errors.New("invalid webhook destination")
	ErrSSRFForbidden      = errors.New("webhook destination IP is prohibited")
	ErrDeliveryFailed     = errors.New("webhook delivery failed")
	ErrReplay             = errors.New("webhook timestamp outside replay window")
)

const (
	WebhookReplayWindow          = 5 * time.Minute
	maxWebhookBodyBytes          = 256 << 10
	strictWebhookPort            = "443"
	ControlledWebhookDestination = "http://receiver:8090/webhook"
)

type WebhookPayload struct {
	Event          string                      `json:"event"`
	DocumentID     string                      `json:"document_id"`
	VersionNumber  int                         `json:"version_number"`
	ApprovedAt     time.Time                   `json:"approved_at"`
	IdempotencyKey string                      `json:"idempotency_key"`
	Normalized     invoices.NormalizedProposal `json:"normalized"`
}

type DeliveryResult struct {
	StatusCode int
	Retryable  bool
	Error      error
}

type WebhookMode string

const (
	WebhookModeStrict     WebhookMode = "strict"
	WebhookModeControlled WebhookMode = "controlled"
)

// WebhookSender owns the destination and secret in worker memory. Neither is
// loaded from an export record or request. Controlled mode is an exact,
// server-configured Compose receiver adapter, not a general private-network
// bypass.
type WebhookSender struct {
	Secret      string
	Destination string
	Mode        WebhookMode
	Client      *http.Client
	Resolver    *net.Resolver
}

var blockedCIDRs []*net.IPNet

func init() {
	for _, value := range []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
		"169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24",
		"192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24",
		"224.0.0.0/4", "240.0.0.0/4", "::/128", "::1/128", "fc00::/7",
		"fe80::/10", "ff00::/8", "2001:db8::/32",
	} {
		_, block, err := net.ParseCIDR(value)
		if err == nil {
			blockedCIDRs = append(blockedCIDRs, block)
		}
	}
}

func NewStrictWebhookSender(secret, destination string) *WebhookSender {
	return newWebhookSender(secret, destination, WebhookModeStrict)
}

func NewControlledWebhookSender(secret, destination string) *WebhookSender {
	return newWebhookSender(secret, destination, WebhookModeControlled)
}

func newWebhookSender(secret, destination string, mode WebhookMode) *WebhookSender {
	s := &WebhookSender{Secret: secret, Destination: destination, Mode: mode, Resolver: net.DefaultResolver}
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, ErrInvalidDestination
			}
			if s.Mode == WebhookModeControlled {
				return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
			}
			ips, err := s.Resolver.LookupIP(ctx, "ip", host)
			if err != nil || len(ips) == 0 {
				return nil, ErrInvalidDestination
			}
			// Validate every answer and connect to the first validated answer. A
			// second DNS answer cannot silently become the dial target.
			for _, ip := range ips {
				if err := ValidateIP(ip); err != nil {
					return nil, err
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
		MaxIdleConns: 10, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 5 * time.Second,
	}
	s.Client = &http.Client{Timeout: 10 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return s
}

func ValidateIP(ip net.IP) error {
	if ip == nil {
		return ErrInvalidDestination
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	for _, block := range blockedCIDRs {
		if block.Contains(ip) {
			return ErrSSRFForbidden
		}
	}
	return nil
}

func (s *WebhookSender) ValidateURL(target string) (*url.URL, error) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, ErrInvalidDestination
	}
	if s.Mode == WebhookModeControlled {
		if target != ControlledWebhookDestination || s.Destination != ControlledWebhookDestination || strings.ToLower(parsed.Scheme) != "http" || parsed.Port() != "8090" {
			return nil, ErrInvalidDestination
		}
		return parsed, nil
	}
	if strings.ToLower(parsed.Scheme) != "https" {
		return nil, ErrInvalidDestination
	}
	if parsed.Port() != "" && parsed.Port() != strictWebhookPort {
		return nil, ErrInvalidDestination
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		if err := ValidateIP(ip); err != nil {
			return nil, err
		}
	}
	return parsed, nil
}

// ComputeSignature signs exactly timestamp + "." + canonical JSON bytes.
func ComputeSignature(secret, timestamp string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func CanonicalPayload(payload WebhookPayload) ([]byte, error) { return json.Marshal(payload) }

// VerifySignature is receiver-side verification. It rejects stale timestamps,
// malformed signatures, and non-constant-time comparisons.
func VerifySignature(secret, signatureHeader, timestampHeader string, body []byte, now time.Time, window time.Duration) error {
	if window <= 0 {
		window = WebhookReplayWindow
	}
	timestamp, err := time.Parse(time.RFC3339, timestampHeader)
	if err != nil || now.Sub(timestamp) > window || timestamp.Sub(now) > window {
		return ErrReplay
	}
	parts := strings.Split(signatureHeader, ",")
	var received string
	for _, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		if ok && strings.TrimSpace(key) == "v1" {
			received = strings.TrimSpace(value)
		}
	}
	expected, err := hex.DecodeString(received)
	if err != nil || len(expected) != sha256.Size {
		return ErrInvalidDestination
	}
	actual, err := hex.DecodeString(ComputeSignature(secret, timestampHeader, body))
	if err != nil || !hmac.Equal(actual, expected) {
		return ErrInvalidDestination
	}
	return nil
}

func (s *WebhookSender) Send(ctx context.Context, payload WebhookPayload) DeliveryResult {
	if strings.TrimSpace(s.Secret) == "" {
		return DeliveryResult{Error: ErrInvalidDestination}
	}
	parsedURL, err := s.ValidateURL(s.Destination)
	if err != nil {
		return DeliveryResult{Error: err}
	}
	bodyBytes, err := CanonicalPayload(payload)
	if err != nil {
		return DeliveryResult{Error: ErrDeliveryFailed}
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	signature := ComputeSignature(s.Secret, timestamp, bodyBytes)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, parsedURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return DeliveryResult{Error: ErrDeliveryFailed}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-InvoiceFlow-Timestamp", timestamp)
	req.Header.Set("X-InvoiceFlow-Idempotency-Key", payload.IdempotencyKey)
	req.Header.Set("X-InvoiceFlow-Signature", "t="+timestamp+",v1="+signature)
	resp, err := s.Client.Do(req)
	if err != nil {
		return DeliveryResult{Retryable: !errors.Is(err, ErrInvalidDestination) && !errors.Is(err, ErrSSRFForbidden), Error: ErrDeliveryFailed}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxWebhookBodyBytes))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return DeliveryResult{StatusCode: resp.StatusCode}
	}
	return DeliveryResult{StatusCode: resp.StatusCode, Retryable: resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests, Error: ErrDeliveryFailed}
}

func SanitizeDestinationLabel(targetURL string) string {
	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" {
		return "Server-configured webhook"
	}
	return fmt.Sprintf("%s webhook", strings.ToUpper(parsed.Scheme))
}
