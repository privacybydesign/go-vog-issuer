package vog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

// escapeQuotes mirrors the escaping mime/multipart applies to filenames.
func escapeQuotes(s string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(s)
}

// ValidationCode is the response code of the GAAV validation service
// (https://validatie.nl), the trust service of the Justitiële Informatiedienst
// that checks the authenticity and integrity of a VOG PDF.
//
// The published API specification (API-specificatie GAAV v1.0) lists the codes
// in a table whose layout is garbled; the meaning below follows the order of
// the descriptions in that table and has been confirmed against the live
// service: a genuine VOG yields 0 and an arbitrary PDF yields 2.
type ValidationCode int

const (
	// CodeAuthentic: "Document is authentiek en integer."
	CodeAuthentic ValidationCode = 0
	// CodeKnownNotIntegral: "Document is bekend, maar niet integer." The
	// document was registered but its content has been altered.
	CodeKnownNotIntegral ValidationCode = 1
	// CodeUnknown: "Het document is niet bekend en kan niet worden
	// gevalideerd."
	CodeUnknown ValidationCode = 2
	// CodeRetry: "De validatie is niet mogelijk, probeer het nogmaals."
	CodeRetry ValidationCode = 3
	// CodeProvenanceError: "Foutmelding ontvangen van provenance server."
	CodeProvenanceError ValidationCode = 4
	// CodeSignatureServerError: "Foutmelding ontvangen van signature
	// validation server."
	CodeSignatureServerError ValidationCode = 5
	// CodeInvalidSignature: "Handtekening is ongeldig."
	CodeInvalidSignature ValidationCode = 6
	// CodeProvenanceStoreError: "De provenance store heeft fouten terug
	// gegeven."
	CodeProvenanceStoreError ValidationCode = 7
)

// Authentic reports whether the document passed validation.
func (c ValidationCode) Authentic() bool { return c == CodeAuthentic }

// DocumentRejected reports whether the service positively rejected the
// document (tampered, unknown or invalid signature). Retrying will not help.
func (c ValidationCode) DocumentRejected() bool {
	switch c {
	case CodeKnownNotIntegral, CodeUnknown, CodeInvalidSignature:
		return true
	}
	return false
}

// Retryable reports whether the failure lies with the validation service and a
// later retry may succeed.
func (c ValidationCode) Retryable() bool {
	switch c {
	case CodeRetry, CodeProvenanceError, CodeSignatureServerError, CodeProvenanceStoreError:
		return true
	}
	return false
}

// Key is a stable machine readable identifier for the code, used by the API
// and translated by the frontend.
func (c ValidationCode) Key() string {
	switch c {
	case CodeAuthentic:
		return "authentic"
	case CodeKnownNotIntegral:
		return "known_not_integral"
	case CodeUnknown:
		return "unknown_document"
	case CodeRetry:
		return "validation_unavailable"
	case CodeProvenanceError:
		return "provenance_error"
	case CodeSignatureServerError:
		return "signature_server_error"
	case CodeInvalidSignature:
		return "invalid_signature"
	case CodeProvenanceStoreError:
		return "provenance_store_error"
	}
	return "unknown_response_code"
}

// Description is the Dutch description from the GAAV API specification.
func (c ValidationCode) Description() string {
	switch c {
	case CodeAuthentic:
		return "Document is authentiek en integer."
	case CodeKnownNotIntegral:
		return "Document is bekend, maar niet integer."
	case CodeUnknown:
		return "Het document is niet bekend en kan niet worden gevalideerd. Neem contact op met de uitgever van het document."
	case CodeRetry:
		return "De validatie is niet mogelijk, probeer het nogmaals. Slaagt de validatie na enkele pogingen nog niet? Neem dan contact op met de uitgever van het document."
	case CodeProvenanceError:
		return "Foutmelding ontvangen van provenance server."
	case CodeSignatureServerError:
		return "Foutmelding ontvangen van signature validation server."
	case CodeInvalidSignature:
		return "Handtekening is ongeldig. Neem contact op met de uitgever van het document."
	case CodeProvenanceStoreError:
		return "De provenance store heeft fouten terug gegeven."
	}
	return fmt.Sprintf("Onbekende responsecode %d.", int(c))
}

// Validator checks a VOG PDF for authenticity and integrity.
type Validator interface {
	Validate(ctx context.Context, pdf []byte, filename string) (ValidationCode, error)
}

// DefaultValidationURL is the production endpoint of the GAAV validation
// service.
const DefaultValidationURL = "https://validatie.nl/api/valideer/"

const (
	// DefaultTimeout bounds a single call to the validation service.
	DefaultTimeout = 30 * time.Second
	// DefaultMaxAttempts is the total number of calls made for one
	// validation when the service keeps failing.
	DefaultMaxAttempts = 3
	// DefaultRetryDelay is the pause before the first retry.
	DefaultRetryDelay = time.Second
)

// RetryPolicy controls how a failed call to the validation service is
// repeated. Only failures on the side of the service are retried: the service
// is unreachable, answers with a 5xx status or returns one of the retryable
// response codes (3, 4, 5 and 7).
//
// A rejection of the document, a client error (4xx), an unparsable answer and
// a timeout are final. The first three do not change on a retry. A service
// that did not answer within the timeout is unlikely to do so a second time,
// and repeating the wait would push the whole upload past the HTTP server's
// write deadline; the frontend retries such an upload later, in view of the
// user.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts, including the first one.
	// Values below 1 mean DefaultMaxAttempts; 1 disables retrying.
	MaxAttempts int
	// Delay is the pause before the first retry. Every following retry waits
	// twice as long. Values of zero or less mean DefaultRetryDelay.
	Delay time.Duration
}

func (p RetryPolicy) withDefaults() RetryPolicy {
	if p.MaxAttempts < 1 {
		p.MaxAttempts = DefaultMaxAttempts
	}
	if p.Delay <= 0 {
		p.Delay = DefaultRetryDelay
	}
	return p
}

// GaavClient validates documents through the GAAV API: a multipart POST of the
// file to /api/valideer/ answered with {"response_code": n}.
type GaavClient struct {
	url    string
	client *http.Client
	retry  RetryPolicy
}

// NewGaavClient creates a client for the given endpoint (DefaultValidationURL
// when empty). The timeout bounds a single attempt (DefaultTimeout when zero);
// retry decides how often a failing attempt is repeated.
func NewGaavClient(url string, timeout time.Duration, retry RetryPolicy) *GaavClient {
	if url == "" {
		url = DefaultValidationURL
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &GaavClient{
		url:    url,
		client: &http.Client{Timeout: timeout},
		retry:  retry.withDefaults(),
	}
}

type gaavResponse struct {
	ResponseCode *int `json:"response_code"`
}

// transientError marks a failure of the validation service itself (unreachable
// or answering with a 5xx status) that may well succeed on a retry.
type transientError struct {
	err error
}

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

// Transient reports whether err is a failure of the validation service that
// may succeed on a retry, as opposed to a problem with the request or with the
// answer.
func Transient(err error) bool {
	var t *transientError
	return errors.As(err, &t)
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// shouldRetry decides whether the outcome of one attempt warrants another.
func shouldRetry(code ValidationCode, err error) bool {
	if err != nil {
		return Transient(err)
	}
	return code.Retryable()
}

// Validate uploads the PDF and returns the validation code. A transport or
// protocol failure is returned as an error; any well formed answer, including
// rejections, is returned as a code. Failures of the service are retried
// according to the client's RetryPolicy before the last outcome is returned.
func (g *GaavClient) Validate(ctx context.Context, pdf []byte, filename string) (ValidationCode, error) {
	if filename == "" {
		filename = "document.pdf"
	}
	body, contentType, err := multipartBody(pdf, filename)
	if err != nil {
		return -1, err
	}

	delay := g.retry.Delay
	for attempt := 1; ; attempt++ {
		code, err := g.validateOnce(ctx, body, contentType)
		if !shouldRetry(code, err) || attempt >= g.retry.MaxAttempts {
			return code, err
		}

		logArgs := []any{"url", g.url, "attempt", attempt, "max_attempts", g.retry.MaxAttempts, "retry_in", delay}
		if err != nil {
			logArgs = append(logArgs, "error", err)
		} else {
			logArgs = append(logArgs, "response_code", int(code), "meaning", code.Key())
		}
		slog.Warn("validation service unavailable, retrying", logArgs...)

		select {
		case <-ctx.Done():
			return code, err
		case <-time.After(delay):
		}
		delay *= 2
	}
}

// multipartBody builds the multipart/form-data body once so that every attempt
// sends the same bytes.
func multipartBody(pdf []byte, filename string) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	// The GAAV service goes by the declared content type of the part: with
	// Go's default application/octet-stream a genuine VOG is answered with
	// response code 2 (unknown document), with application/pdf it validates.
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeQuotes(filename)))
	partHeader.Set("Content-Type", "application/pdf")
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return nil, "", fmt.Errorf("failed to build multipart body: %w", err)
	}
	if _, err := part.Write(pdf); err != nil {
		return nil, "", fmt.Errorf("failed to write multipart body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to finish multipart body: %w", err)
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

// validateOnce performs a single call to the validation service.
func (g *GaavClient) validateOnce(ctx context.Context, body []byte, contentType string) (ValidationCode, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url, bytes.NewReader(body))
	if err != nil {
		return -1, fmt.Errorf("failed to create validation request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		err = fmt.Errorf("validation request failed: %w", err)
		if ctx.Err() != nil || isTimeout(err) {
			return -1, err
		}
		return -1, &transientError{err}
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return -1, fmt.Errorf("failed to read validation response: %w", err)
	}
	if resp.StatusCode >= http.StatusInternalServerError {
		return -1, &transientError{fmt.Errorf("validation service returned status %d: %s", resp.StatusCode, bytes.TrimSpace(respBody))}
	}
	if resp.StatusCode != http.StatusOK {
		return -1, fmt.Errorf("validation service returned status %d: %s", resp.StatusCode, bytes.TrimSpace(respBody))
	}

	var parsed gaavResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return -1, fmt.Errorf("failed to decode validation response %q: %w", bytes.TrimSpace(respBody), err)
	}
	if parsed.ResponseCode == nil {
		return -1, fmt.Errorf("validation response lacks response_code: %s", bytes.TrimSpace(respBody))
	}
	return ValidationCode(*parsed.ResponseCode), nil
}

// Timeout is the bound on a single call to the validation service.
func (g *GaavClient) Timeout() time.Duration { return g.client.Timeout }

// Retry is the effective retry policy, with defaults applied.
func (g *GaavClient) Retry() RetryPolicy { return g.retry }
