package vog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
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

// GaavClient validates documents through the GAAV API: a multipart POST of the
// file to /api/valideer/ answered with {"response_code": n}.
type GaavClient struct {
	url    string
	client *http.Client
}

// NewGaavClient creates a client for the given endpoint (DefaultValidationURL
// when empty).
func NewGaavClient(url string, timeout time.Duration) *GaavClient {
	if url == "" {
		url = DefaultValidationURL
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &GaavClient{
		url:    url,
		client: &http.Client{Timeout: timeout},
	}
}

type gaavResponse struct {
	ResponseCode *int `json:"response_code"`
}

// Validate uploads the PDF and returns the validation code. A transport or
// protocol failure is returned as an error; any well formed answer, including
// rejections, is returned as a code.
func (g *GaavClient) Validate(ctx context.Context, pdf []byte, filename string) (ValidationCode, error) {
	if filename == "" {
		filename = "document.pdf"
	}

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
		return -1, fmt.Errorf("failed to build multipart body: %w", err)
	}
	if _, err := part.Write(pdf); err != nil {
		return -1, fmt.Errorf("failed to write multipart body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return -1, fmt.Errorf("failed to finish multipart body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url, &body)
	if err != nil {
		return -1, fmt.Errorf("failed to create validation request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return -1, fmt.Errorf("validation request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return -1, fmt.Errorf("failed to read validation response: %w", err)
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
