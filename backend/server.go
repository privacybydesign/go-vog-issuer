package main

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"go-vog-issuer/identity"
	"go-vog-issuer/models"
	"go-vog-issuer/vog"

	"github.com/gorilla/mux"
	"github.com/privacybydesign/irmago/irma"
)

//go:embed docs/swagger.yaml
var swaggerSpec []byte

//go:embed docs/redoc.html
var redocHTML []byte

// Error keys returned in the "error" field of an ErrorResponse.
const (
	ErrorInternal          = "error:internal"
	ErrorMethodNotAllowed  = "error:method-not-allowed"
	ErrorInvalidRequest    = "error:invalid-request"
	ErrorFileMissing       = "error:file-missing"
	ErrorFileTooLarge      = "error:file-too-large"
	ErrorNotAPdf           = "error:not-a-pdf"
	ErrorNotAVog           = "error:not-a-vog"
	ErrorValidationFailed  = "error:validation-failed"
	ErrorValidationService = "error:validation-service-unavailable"
	ErrorUnknownSession    = "error:unknown-session"
	ErrorDisclosureNotDone = "error:disclosure-not-done"
	ErrorDisclosureInvalid = "error:disclosure-invalid"
	ErrorIdentityMismatch  = "error:identity-mismatch"
	ErrorIrmaServer        = "error:irma-server"
)

const DefaultMaxUploadSize = 5 << 20 // 5 MiB

type ServerConfig struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	UseTls         bool   `json:"use_tls,omitempty"`
	TlsPrivKeyPath string `json:"tls_priv_key_path,omitempty"`
	TlsCertPath    string `json:"tls_cert_path,omitempty"`
	// Serve the API documentation on /api/docs and /api/docs/swagger.yaml.
	// Disabled unless explicitly enabled, so the docs stay off in production.
	EnableApiDocs bool `json:"enable_api_docs,omitempty"`
}

type ServerState struct {
	irmaServerURL       string
	sessionStorage      SessionStorage
	jwtCreator          JwtCreator
	validator           vog.Validator
	parser              vog.Parser
	irmaClient          IrmaClient
	identityCredentials IdentityCredentials
	maxUploadSize       int64
}

type SpaHandler struct {
	staticPath string
	indexPath  string
}

type Server struct {
	server *http.Server
	config ServerConfig
}

func (s *Server) ListenAndServe() error {
	if s.config.UseTls {
		slog.Info("Starting server with TLS", "host", s.config.Host, "port", s.config.Port, "cert", s.config.TlsCertPath, "key", s.config.TlsPrivKeyPath)
		return s.server.ListenAndServeTLS(s.config.TlsCertPath, s.config.TlsPrivKeyPath)
	}
	slog.Info("Starting server without TLS", "host", s.config.Host, "port", s.config.Port)
	return s.server.ListenAndServe()
}

func (s *Server) Stop() error {
	slog.Info("Shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := s.server.Shutdown(ctx)
	if err != nil {
		slog.Error("Error during server shutdown", "error", err)
	} else {
		slog.Info("Server shut down successfully")
	}
	return err
}

// ServeHTTP inspects the URL path to locate a file within the static dir
// on the SPA handler. If a file is found, it will be served. If not, the
// file located at the index path on the SPA handler will be served. This
// is suitable behavior for serving an SPA (single page application).
// https://github.com/gorilla/mux?tab=readme-ov-file#serving-single-page-applications
func (h SpaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	slog.Debug("SPA handler serving request", "path", r.URL.Path)
	// Join internally call path.Clean to prevent directory traversal
	path := filepath.Join(h.staticPath, r.URL.Path)
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	}

	if err != nil {
		// Log the raw OS error server-side and return a generic 500 so we don't
		// leak filesystem paths or OS internals to the client.
		slog.Error("static file error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if fi.IsDir() {
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	}

	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
}

func NewServer(state *ServerState, config ServerConfig) (*Server, error) {
	slog.Info("Creating new server", "host", config.Host, "port", config.Port, "tls", config.UseTls)
	if state.maxUploadSize <= 0 {
		state.maxUploadSize = DefaultMaxUploadSize
	}
	router := mux.NewRouter()

	router.HandleFunc("/api/health", handleHealth).Methods(http.MethodGet)

	router.HandleFunc("/api/vog/upload", func(w http.ResponseWriter, r *http.Request) {
		handleUpload(state, w, r)
	})
	router.HandleFunc("/api/vog/start-disclosure", func(w http.ResponseWriter, r *http.Request) {
		handleStartDisclosure(state, w, r)
	})
	router.HandleFunc("/api/vog/issue", func(w http.ResponseWriter, r *http.Request) {
		handleIssue(state, w, r)
	})

	// API Documentation
	if config.EnableApiDocs {
		router.HandleFunc("/api/docs", HandleRedocRequest).Methods(http.MethodGet)
		router.HandleFunc("/api/docs/swagger.yaml", HandleSwaggerRequest).Methods(http.MethodGet)
		slog.Info("API documentation enabled", "path", "/api/docs")
	} else {
		slog.Info("API documentation disabled")
	}

	spa := SpaHandler{staticPath: "../frontend/build", indexPath: "index.html"}
	router.PathPrefix("/").Handler(spa)

	addr := fmt.Sprintf("%v:%v", config.Host, config.Port)
	srv := &http.Server{
		Handler: router,
		Addr:    addr,
		// The upload handler waits for the external validation service, so
		// allow more than the usual 15 seconds before the write deadline hits.
		WriteTimeout: 60 * time.Second,
		ReadTimeout:  30 * time.Second,
	}

	slog.Info("Server created successfully", "address", addr)
	return &Server{
		server: srv,
		config: config,
	}, nil
}

// handleHealth returns the health status of the service
// @Summary Health check
// @Description Returns the health status of the API service
// @Tags Health
// @Produce json
// @Success 200 {object} models.HealthResponse
// @Router /health [get]
func handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := writeJSON(w, http.StatusOK, models.HealthResponse{Ok: true}); err != nil {
		slog.Error("failed to write health response", "error", err)
	}
}

// handleUpload validates and parses an uploaded VOG
// @Summary Upload and validate a VOG
// @Description Accepts a VOG PDF (multipart form field "file"), checks its authenticity and integrity with the GAAV validation service of the Justitiële Informatiedienst (https://validatie.nl) and reads the printed data (name, date of birth, purpose and screening profile codes). On success a session is created that must be used to disclose the holder's identity and to obtain the credential. The session expires after one hour.
// @Tags VOG
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "The VOG PDF as received from Justis"
// @Success 200 {object} models.UploadResponse
// @Failure 400 {object} models.ErrorResponse "file missing, not a PDF or not a VOG"
// @Failure 413 {object} models.ErrorResponse "file too large"
// @Failure 422 {object} models.ErrorResponse "the validation service rejected the document (tampered, unknown or invalid signature)"
// @Failure 503 {object} models.ErrorResponse "the validation service is unavailable"
// @Failure 500 {object} models.ErrorResponse
// @Router /vog/upload [post]
func handleUpload(state *ServerState, w http.ResponseWriter, r *http.Request) {
	defer closeRequestBody(r)

	if !requirePOST(w, r) {
		return
	}
	const endpoint = "vog/upload"
	slog.Info("Received request", "endpoint", endpoint)

	r.Body = http.MaxBytesReader(w, r.Body, state.maxUploadSize)
	if err := r.ParseMultipartForm(state.maxUploadSize); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			respondWithErr(w, http.StatusRequestEntityTooLarge, ErrorFileTooLarge, "uploaded file exceeds the size limit", err, "endpoint", endpoint)
			return
		}
		respondWithErr(w, http.StatusBadRequest, ErrorInvalidRequest, "failed to parse multipart form", err, "endpoint", endpoint)
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		respondWithErr(w, http.StatusBadRequest, ErrorFileMissing, "multipart field 'file' is missing", err, "endpoint", endpoint)
		return
	}
	defer func() { _ = file.Close() }()

	pdf, err := io.ReadAll(io.LimitReader(file, state.maxUploadSize+1))
	if err != nil {
		respondWithErr(w, http.StatusBadRequest, ErrorInvalidRequest, "failed to read uploaded file", err, "endpoint", endpoint)
		return
	}
	if int64(len(pdf)) > state.maxUploadSize {
		respondWithErr(w, http.StatusRequestEntityTooLarge, ErrorFileTooLarge, "uploaded file exceeds the size limit", nil, "endpoint", endpoint)
		return
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		respondWithErr(w, http.StatusBadRequest, ErrorNotAPdf, "uploaded file is not a PDF", nil, "endpoint", endpoint)
		return
	}

	// Authenticity and integrity first: only a genuine VOG is worth parsing.
	code, err := state.validator.Validate(r.Context(), pdf, header.Filename)
	if err != nil {
		respondWithJSONErr(w, http.StatusServiceUnavailable, models.ErrorResponse{
			Error:   ErrorValidationService,
			Message: "the validation service could not be reached",
		}, "validation service call failed", err, "endpoint", endpoint)
		return
	}
	validation := validationInfo(code)
	if !code.Authentic() {
		status := http.StatusUnprocessableEntity
		if code.Retryable() {
			status = http.StatusServiceUnavailable
		}
		respondWithJSONErr(w, status, models.ErrorResponse{
			Error:      ErrorValidationFailed,
			Message:    "the document was not accepted by the validation service",
			Validation: &validation,
		}, "document rejected by validation service", fmt.Errorf("response code %d (%s)", int(code), code.Key()), "endpoint", endpoint)
		return
	}

	doc, err := state.parser.Parse(pdf)
	if err != nil {
		if errors.Is(err, vog.ErrNotAVog) {
			respondWithErr(w, http.StatusBadRequest, ErrorNotAVog, "uploaded document is not a VOG", err, "endpoint", endpoint)
			return
		}
		respondWithErr(w, http.StatusInternalServerError, ErrorInternal, "failed to parse VOG", err, "endpoint", endpoint)
		return
	}

	sessionId := GenerateSessionId()
	if sessionId == "" {
		respondWithErr(w, http.StatusInternalServerError, ErrorInternal, "failed to generate session ID", fmt.Errorf("failed to generate session ID"))
		return
	}
	session := &Session{
		Id:             sessionId,
		CreatedAt:      time.Now(),
		Stage:          StageValidated,
		Document:       doc,
		ValidationCode: code,
	}
	if err := state.sessionStorage.Store(session); err != nil {
		respondWithErr(w, http.StatusInternalServerError, ErrorInternal, "failed to store session", err, "endpoint", endpoint)
		return
	}

	response := models.UploadResponse{
		SessionId:  sessionId,
		Validation: validation,
		Document:   documentInfo(doc),
	}
	if err := writeJSON(w, http.StatusOK, response); err != nil {
		respondWithErr(w, http.StatusInternalServerError, ErrorInternal, "failed to marshal response message", err)
		return
	}
	slog.Info("VOG validated and parsed", "session_id", sessionId, "reference_number", doc.ReferenceNumber, "profile_codes", doc.ProfileCodes)
}

// handleStartDisclosure starts the identity disclosure session
// @Summary Start identity disclosure
// @Description Starts a Yivi disclosure session in which the holder proves their identity with one of four credentials: BRP personal data (gemeente.personalData), passport, ID card or driving licence. The Yivi app lets the user pick. The response is the IRMA session package that yivi-frontend consumes directly (use it as the response of the `session.start` request with the default mapping and `result: false`). The requestor token is kept server side.
// @Tags VOG
// @Accept json
// @Produce json
// @Param request body models.SessionRequest true "Session from /vog/upload"
// @Success 200 {object} models.DisclosureSessionResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse "unknown or expired session"
// @Failure 502 {object} models.ErrorResponse "the IRMA server did not accept the session"
// @Failure 500 {object} models.ErrorResponse
// @Router /vog/start-disclosure [post]
func handleStartDisclosure(state *ServerState, w http.ResponseWriter, r *http.Request) {
	defer closeRequestBody(r)

	if !requirePOST(w, r) {
		return
	}
	const endpoint = "vog/start-disclosure"
	slog.Info("Received request", "endpoint", endpoint)

	request, err := decodeSessionRequest(r)
	if err != nil {
		respondWithErr(w, http.StatusBadRequest, ErrorInvalidRequest, "failed to decode request", err, "endpoint", endpoint)
		return
	}
	session, err := state.sessionStorage.Retrieve(request.SessionId)
	if err != nil {
		respondWithErr(w, http.StatusNotFound, ErrorUnknownSession, "unknown session", err, "endpoint", endpoint, "session_id", request.SessionId)
		return
	}

	signedJwt, err := state.jwtCreator.CreateDisclosureJwt()
	if err != nil {
		respondWithErr(w, http.StatusInternalServerError, ErrorInternal, "failed to create disclosure jwt", err, "endpoint", endpoint, "session_id", session.Id)
		return
	}
	pkg, err := state.irmaClient.StartSession(r.Context(), signedJwt)
	if err != nil {
		respondWithErr(w, http.StatusBadGateway, ErrorIrmaServer, "failed to start disclosure session on irma server", err, "endpoint", endpoint, "session_id", session.Id)
		return
	}

	session.Stage = StageDisclosing
	session.IrmaToken = string(pkg.Token)
	if err := state.sessionStorage.Store(session); err != nil {
		respondWithErr(w, http.StatusInternalServerError, ErrorInternal, "failed to store session", err, "endpoint", endpoint, "session_id", session.Id)
		return
	}

	response := models.DisclosureSessionResponse{
		SessionPtr: models.SessionPointer{
			U:      pkg.SessionPtr.URL,
			Irmaqr: string(pkg.SessionPtr.Type),
		},
		FrontendRequest: pkg.FrontendRequest,
	}
	if err := writeJSON(w, http.StatusOK, response); err != nil {
		respondWithErr(w, http.StatusInternalServerError, ErrorInternal, "failed to marshal response message", err)
		return
	}
	slog.Info("Disclosure session started", "session_id", session.Id)
}

// handleIssue verifies the disclosed identity and issues the VOG credential
// @Summary Verify identity and issue VOG credential
// @Description Fetches the result of the disclosure session, compares the disclosed name and date of birth with the person named on the VOG and, when they match, returns a signed IRMA issuance request for the VOG credential (IRMA and SD-JWT VC formats, issued over the IRMA protocol). The credential carries the printed data plus one yes/no attribute per function aspect of the general screening profile (aspect11 ... aspect91). On a mismatch the disclosure is discarded and the user may disclose again with another credential; the uploaded VOG stays available until the session expires.
// @Tags VOG
// @Accept json
// @Produce json
// @Param request body models.SessionRequest true "Session from /vog/upload"
// @Success 200 {object} models.IssuanceResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse "the disclosed identity does not match the VOG, or the disclosure proof is invalid"
// @Failure 404 {object} models.ErrorResponse "unknown or expired session"
// @Failure 409 {object} models.ErrorResponse "the disclosure session has not finished (or was not started)"
// @Failure 502 {object} models.ErrorResponse "the IRMA server could not be reached"
// @Failure 500 {object} models.ErrorResponse
// @Router /vog/issue [post]
func handleIssue(state *ServerState, w http.ResponseWriter, r *http.Request) {
	defer closeRequestBody(r)

	if !requirePOST(w, r) {
		return
	}
	const endpoint = "vog/issue"
	slog.Info("Received request", "endpoint", endpoint)

	request, err := decodeSessionRequest(r)
	if err != nil {
		respondWithErr(w, http.StatusBadRequest, ErrorInvalidRequest, "failed to decode request", err, "endpoint", endpoint)
		return
	}
	session, err := state.sessionStorage.Retrieve(request.SessionId)
	if err != nil {
		respondWithErr(w, http.StatusNotFound, ErrorUnknownSession, "unknown session", err, "endpoint", endpoint, "session_id", request.SessionId)
		return
	}
	if session.Stage != StageDisclosing || session.IrmaToken == "" {
		respondWithErr(w, http.StatusConflict, ErrorDisclosureNotDone, "disclosure session was not started", nil, "endpoint", endpoint, "session_id", session.Id)
		return
	}

	result, err := state.irmaClient.GetSessionResult(r.Context(), irma.RequestorToken(session.IrmaToken))
	if err != nil {
		respondWithErr(w, http.StatusBadGateway, ErrorIrmaServer, "failed to fetch disclosure result", err, "endpoint", endpoint, "session_id", session.Id)
		return
	}
	if result.Status != irma.ServerStatusDone {
		respondWithErr(w, http.StatusConflict, ErrorDisclosureNotDone, "disclosure session not finished", fmt.Errorf("status %s", result.Status), "endpoint", endpoint, "session_id", session.Id)
		return
	}
	if result.ProofStatus != irma.ProofStatusValid {
		resetDisclosure(state, session)
		respondWithErr(w, http.StatusForbidden, ErrorDisclosureInvalid, "disclosure proof invalid", fmt.Errorf("proof status %s", result.ProofStatus), "endpoint", endpoint, "session_id", session.Id)
		return
	}

	disclosed, err := ExtractIdentity(result.Disclosed, state.identityCredentials)
	if err != nil {
		resetDisclosure(state, session)
		respondWithErr(w, http.StatusForbidden, ErrorDisclosureInvalid, "disclosed attributes unusable", err, "endpoint", endpoint, "session_id", session.Id)
		return
	}

	vogPerson := identity.Person{
		GivenNames:  session.Document.GivenNames,
		Surname:     session.Document.Surname,
		Prefix:      session.Document.Prefix,
		DateOfBirth: session.Document.DateOfBirth.Format(DATE_FORMAT_CYMD),
	}
	match := identity.Match(vogPerson, disclosed.Person)
	matchInfo := models.IdentityMatchInfo{
		Source:           disclosed.Source,
		Matched:          match.Matched,
		DateOfBirthMatch: match.DateOfBirthMatch,
		SurnameMatch:     match.SurnameMatch,
		GivenNamesMatch:  match.GivenNamesMatch,
		Reasons:          match.Reasons,
	}
	if !match.Matched {
		// The disclosure is spent; the user may try again with another
		// credential, so keep the validated VOG around.
		resetDisclosure(state, session)
		respondWithJSONErr(w, http.StatusForbidden, models.ErrorResponse{
			Error:    ErrorIdentityMismatch,
			Message:  "the disclosed identity does not match the person named on the VOG",
			Identity: &matchInfo,
		}, "identity mismatch", fmt.Errorf("%v", match.Reasons), "endpoint", endpoint, "session_id", session.Id, "source", disclosed.Source)
		return
	}

	signedJwt, err := state.jwtCreator.CreateIssuanceJwt(session.Document, disclosed.Source)
	if err != nil {
		respondWithErr(w, http.StatusInternalServerError, ErrorInternal, "failed to create issuance jwt", err, "endpoint", endpoint, "session_id", session.Id)
		return
	}

	// Consume the session before writing the response so the VOG cannot be
	// issued twice even if the write below fails.
	removeSession(state.sessionStorage, session.Id)

	response := models.IssuanceResponse{
		Jwt:           signedJwt,
		IrmaServerURL: state.irmaServerURL,
		Identity:      matchInfo,
	}
	if err := writeJSON(w, http.StatusOK, response); err != nil {
		respondWithErr(w, http.StatusInternalServerError, ErrorInternal, "failed to marshal response message", err)
		return
	}
	slog.Info("VOG credential issued", "session_id", session.Id, "source", disclosed.Source)
}

// resetDisclosure forgets the (spent) disclosure session so the user can
// disclose again, keeping the validated VOG.
func resetDisclosure(state *ServerState, session *Session) {
	session.Stage = StageValidated
	session.IrmaToken = ""
	if err := state.sessionStorage.Store(session); err != nil {
		slog.Error("failed to reset disclosure state", "error", err, "session_id", session.Id)
	}
}

// removeSession deletes the session, logging (not reporting) failures: a
// removal failure must not alter the response.
func removeSession(storage SessionStorage, sessionId string) {
	if err := storage.Remove(sessionId); err != nil {
		slog.Error("failed to remove session", "error", err, "session_id", sessionId)
	}
}

func decodeSessionRequest(r *http.Request) (models.SessionRequest, error) {
	var request models.SessionRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&request); err != nil {
		return request, fmt.Errorf("decode request body: %w", err)
	}
	if request.SessionId == "" {
		return request, fmt.Errorf("session_id is required")
	}
	return request, nil
}

func validationInfo(code vog.ValidationCode) models.ValidationInfo {
	return models.ValidationInfo{
		Code:        int(code),
		Key:         code.Key(),
		Description: code.Description(),
		Authentic:   code.Authentic(),
		Retryable:   code.Retryable(),
	}
}

func documentInfo(doc *vog.Document) models.DocumentInfo {
	profiles := make([]models.ProfileInfo, 0, len(doc.ProfileCodes))
	for _, code := range doc.ProfileCodes {
		info := models.ProfileInfo{Code: code}
		if aspect, ok := vog.LookupFunctionAspect(code); ok {
			info.RiskArea = aspect.RiskArea
			info.Description = aspect.Description
			info.DescriptionEN = aspect.DescriptionEN
		} else {
			info.Description = vog.DescribeCode(code)
			info.DescriptionEN = vog.DescribeCodeEN(code)
		}
		profiles = append(profiles, info)
	}
	return models.DocumentInfo{
		ReferenceNumber: doc.ReferenceNumber,
		IssueDate:       doc.IssueDate.Format(DATE_FORMAT_CYMD),
		Surname:         doc.Surname,
		Prefix:          doc.Prefix,
		GivenNames:      doc.GivenNames,
		DateOfBirth:     doc.DateOfBirth.Format(DATE_FORMAT_CYMD),
		PlaceOfBirth:    doc.PlaceOfBirth,
		CountryOfBirth:  doc.CountryOfBirth,
		Purpose:         doc.Purpose,
		ProfileCodes:    doc.ProfileCodes,
		Profiles:        profiles,
	}
}

func HandleRedocRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(redocHTML); err != nil {
		slog.Error("failed to write redoc html", "error", err)
	}
}

func HandleSwaggerRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if _, err := w.Write(swaggerSpec); err != nil {
		slog.Error("failed to write swagger spec", "error", err)
	}
}

func GenerateSessionId() string {
	sessionId := make([]byte, 16)
	if _, err := rand.Read(sessionId); err != nil {
		slog.Error("failed to generate session ID", "error", err)
		return ""
	}
	return fmt.Sprintf("%x", sessionId)
}

// respondWithErr writes an ErrorResponse with the given key.
func respondWithErr(w http.ResponseWriter, code int, errorKey string, logMsg string, e error, extras ...any) {
	respondWithJSONErr(w, code, models.ErrorResponse{Error: errorKey, Message: logMsg}, logMsg, e, extras...)
}

func respondWithJSONErr(w http.ResponseWriter, code int, body models.ErrorResponse, logMsg string, e error, extras ...any) {
	args := []any{"error", e, "status_code", code, "error_key", body.Error}
	args = append(args, extras...)
	slog.Error(logMsg, args...)
	if err := writeJSON(w, code, body); err != nil {
		slog.Error("failed to write error response", "error", err)
	}
}

// helpers ------------

func closeRequestBody(r *http.Request) {
	if err := r.Body.Close(); err != nil {
		slog.Error("failed to close request body", "error", err)
	}
}

func requirePOST(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		slog.Debug("Non-POST request rejected", "method", r.Method, "path", r.URL.Path)
		respondWithErr(w, http.StatusMethodNotAllowed, ErrorMethodNotAllowed, "method not allowed", nil)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		slog.Error("Failed to marshal JSON payload", "error", err)
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err = w.Write(payload); err != nil {
		slog.Error("failed to write body to http response", "error", err)
	}
	return nil
}
