// Package models contains the request and response bodies of the VOG issuer
// API.
package models

// ErrorResponse is returned on every failed request.
type ErrorResponse struct {
	// Stable machine readable error key, e.g. "error:validation-failed"
	Error string `json:"error" example:"error:validation-failed"`
	// Human readable explanation (English)
	Message string `json:"message,omitempty" example:"the document was rejected by the validation service"`
	// Validation outcome when the error concerns the GAAV validation
	Validation *ValidationInfo `json:"validation,omitempty"`
	// Identity comparison outcome when the error concerns the identity match
	Identity *IdentityMatchInfo `json:"identity,omitempty"`
}

// ValidationInfo describes the answer of the GAAV validation service.
type ValidationInfo struct {
	// Raw response code of https://validatie.nl (0 = authentic and integral)
	Code int `json:"code" example:"0"`
	// Stable key for the code: authentic, known_not_integral, unknown_document, validation_unavailable, provenance_error, signature_server_error, invalid_signature, provenance_store_error
	Key string `json:"key" example:"authentic"`
	// Dutch description from the GAAV API specification
	Description string `json:"description" example:"Document is authentiek en integer."`
	// True when the document is authentic and integral
	Authentic bool `json:"authentic" example:"true"`
	// True when the failure is on the side of the validation service and retrying may help
	Retryable bool `json:"retryable" example:"false"`
}

// ProfileInfo describes one screening profile code found on the VOG.
type ProfileInfo struct {
	// Two digit code as printed on the VOG
	Code string `json:"code" example:"84"`
	// Risk area (Dutch)
	RiskArea string `json:"risk_area" example:"Personen"`
	// Description (Dutch)
	Description string `json:"description" example:"Belast zijn met de zorg voor minderjarigen"`
	// Description (English)
	DescriptionEN string `json:"description_en" example:"Being responsible for the care of minors"`
}

// DocumentInfo is the data read from the VOG, echoed back so the user can
// check it before continuing.
type DocumentInfo struct {
	ReferenceNumber string `json:"reference_number" example:"9999012026032500922"`
	// Issue date, YYYY-MM-DD
	IssueDate  string `json:"issue_date" example:"2026-03-25"`
	Surname    string `json:"surname" example:"Jansen"`
	Prefix     string `json:"prefix" example:"van"`
	GivenNames string `json:"given_names" example:"Jan Willem"`
	// Date of birth, YYYY-MM-DD
	DateOfBirth    string `json:"date_of_birth" example:"1991-05-14"`
	PlaceOfBirth   string `json:"place_of_birth" example:"Barneveld"`
	CountryOfBirth string `json:"country_of_birth" example:"Nederland"`
	// The function or purpose the VOG was requested for
	Purpose string `json:"purpose" example:"Vrijwilliger bij Sportvereniging"`
	// Screening profile codes as printed on the VOG
	ProfileCodes []string      `json:"profile_codes" example:"84,85"`
	Profiles     []ProfileInfo `json:"profiles"`
}

// UploadResponse is returned after a VOG has been validated and parsed.
type UploadResponse struct {
	// Session identifier to use in the follow-up requests (32 hex characters)
	SessionId  string         `json:"session_id" example:"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"`
	Validation ValidationInfo `json:"validation"`
	Document   DocumentInfo   `json:"document"`
}

// SessionRequest identifies the upload session for the follow-up steps.
type SessionRequest struct {
	// Session identifier from /vog/upload
	SessionId string `json:"session_id" example:"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"`
}

// SessionPointer tells the Yivi app where to find the session (the IRMA "Qr"
// structure).
type SessionPointer struct {
	// Session URL on the IRMA server
	U string `json:"u" example:"https://irma.example.com/irma/session/abc123"`
	// Session type
	Irmaqr string `json:"irmaqr" example:"disclosing"`
}

// DisclosureSessionResponse is the IRMA session package for the identity
// disclosure, consumed directly by yivi-frontend.
type DisclosureSessionResponse struct {
	SessionPtr SessionPointer `json:"sessionPtr"`
	// Frontend session request (pairing options) as produced by the IRMA server
	FrontendRequest any `json:"frontendRequest" swaggertype:"object"`
}

// IdentityMatchInfo explains the comparison between the VOG and the disclosed
// identity.
type IdentityMatchInfo struct {
	// Which credential the identity came from: brp, passport, id_card or driving_licence
	Source           string   `json:"source" example:"passport"`
	Matched          bool     `json:"matched" example:"true"`
	DateOfBirthMatch bool     `json:"date_of_birth_match" example:"true"`
	SurnameMatch     bool     `json:"surname_match" example:"true"`
	GivenNamesMatch  bool     `json:"given_names_match" example:"true"`
	Reasons          []string `json:"reasons,omitempty" example:"surname differs"`
}

// IssuanceResponse contains the JWT and IRMA server URL for credential issuance
type IssuanceResponse struct {
	// Signed JWT containing the IRMA issuance request
	Jwt string `json:"jwt" example:"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."`
	// URL of the IRMA server for credential issuance
	IrmaServerURL string `json:"irma_server_url" example:"https://irma.example.com"`
	// Outcome of the identity comparison that authorised the issuance
	Identity IdentityMatchInfo `json:"identity"`
}

// HealthResponse contains the health status of the service
type HealthResponse struct {
	// True if the service is healthy
	Ok bool `json:"ok" example:"true"`
}
