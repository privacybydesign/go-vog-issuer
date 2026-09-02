package main

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"go-vog-issuer/models"
	"go-vog-issuer/vog"

	"github.com/privacybydesign/irmago/irma"
	"github.com/stretchr/testify/require"
)

const (
	uploadEndpoint     = "/api/vog/upload"
	disclosureEndpoint = "/api/vog/start-disclosure"
	issueEndpoint      = "/api/vog/issue"
)

var fakePdf = []byte("%PDF-1.5 fake vog content\n%%EOF\n")

func uploadOk(t *testing.T) *models.UploadResponse {
	t.Helper()
	resp, body, upload := postFile[models.UploadResponse](t, fmt.Sprintf(testHost, uploadEndpoint), "file", "vog.pdf", fakePdf)
	mustStatus(t, resp, http.StatusOK, body)
	require.NotEmpty(t, upload.SessionId)
	return upload
}

func startDisclosureOk(t *testing.T, sessionId string) {
	t.Helper()
	resp, body, _ := postJSON[models.DisclosureSessionResponse](t, fmt.Sprintf(testHost, disclosureEndpoint), models.SessionRequest{SessionId: sessionId})
	mustStatus(t, resp, http.StatusOK, body)
}

func TestHealth(t *testing.T) {
	startTestServer(t, defaultDeps())
	resp, err := http.Get(fmt.Sprintf(testHost, "/api/health"))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestUploadHappyPath(t *testing.T) {
	deps := defaultDeps()
	startTestServer(t, deps)

	upload := uploadOk(t)
	require.True(t, upload.Validation.Authentic)
	require.Equal(t, 0, upload.Validation.Code)
	require.Equal(t, "authentic", upload.Validation.Key)
	require.Equal(t, "9999012026032500922", upload.Document.ReferenceNumber)
	require.Equal(t, "2026-03-25", upload.Document.IssueDate)
	require.Equal(t, "Berg", upload.Document.Surname)
	require.Equal(t, "van der", upload.Document.Prefix)
	require.Equal(t, "Anna Maria", upload.Document.GivenNames)
	require.Equal(t, "1980-02-03", upload.Document.DateOfBirth)
	require.Equal(t, []string{"84", "85"}, upload.Document.ProfileCodes)
	require.Len(t, upload.Document.Profiles, 2)
	require.Equal(t, "84", upload.Document.Profiles[0].Code)
	require.Equal(t, "Personen", upload.Document.Profiles[0].RiskArea)
	require.Equal(t, "Belast zijn met de zorg voor minderjarigen", upload.Document.Profiles[0].Description)
	require.Equal(t, "Being responsible for the care of minors", upload.Document.Profiles[0].DescriptionEN)

	require.Equal(t, 1, deps.validator.calls)
	require.Equal(t, fakePdf, deps.validator.last)

	session, err := deps.storage.Retrieve(upload.SessionId)
	require.NoError(t, err)
	require.Equal(t, StageValidated, session.Stage)
	require.Equal(t, vog.CodeAuthentic, session.ValidationCode)
}

func TestUploadRejectsBadRequests(t *testing.T) {
	deps := defaultDeps()
	startTestServer(t, deps)
	url := fmt.Sprintf(testHost, uploadEndpoint)

	t.Run("missing file field", func(t *testing.T) {
		resp, body, errResp := postFile[models.ErrorResponse](t, url, "", "", nil)
		mustStatus(t, resp, http.StatusBadRequest, body)
		require.Equal(t, ErrorFileMissing, errResp.Error)
	})

	t.Run("not a pdf", func(t *testing.T) {
		resp, body, errResp := postFile[models.ErrorResponse](t, url, "file", "x.txt", []byte("hello"))
		mustStatus(t, resp, http.StatusBadRequest, body)
		require.Equal(t, ErrorNotAPdf, errResp.Error)
	})

	t.Run("pdf header without trailer", func(t *testing.T) {
		resp, body, errResp := postFile[models.ErrorResponse](t, url, "file", "x.pdf", []byte("%PDF-1.5\nnot really a pdf"))
		mustStatus(t, resp, http.StatusBadRequest, body)
		require.Equal(t, ErrorNotAPdf, errResp.Error)
	})

	t.Run("wrong extension", func(t *testing.T) {
		resp, body, errResp := postFile[models.ErrorResponse](t, url, "file", "x.exe", []byte("%PDF-1.5\n%%EOF\n"))
		mustStatus(t, resp, http.StatusBadRequest, body)
		require.Equal(t, ErrorNotAPdf, errResp.Error)
	})

	t.Run("wrong content type", func(t *testing.T) {
		resp, body, errResp := postFileWithType[models.ErrorResponse](t, url, "x.pdf", "image/png", []byte("%PDF-1.5\n%%EOF\n"))
		mustStatus(t, resp, http.StatusBadRequest, body)
		require.Equal(t, ErrorNotAPdf, errResp.Error)
	})

	t.Run("too large", func(t *testing.T) {
		big := append([]byte("%PDF"), make([]byte, 2<<20)...)
		resp, body, errResp := postFile[models.ErrorResponse](t, url, "file", "big.pdf", big)
		mustStatus(t, resp, http.StatusRequestEntityTooLarge, body)
		require.Equal(t, ErrorFileTooLarge, errResp.Error)
	})

	t.Run("get not allowed", func(t *testing.T) {
		resp, err := http.Get(url)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})

	// The validator must never have been called for these.
	require.Equal(t, 0, deps.validator.calls)
}

func TestUploadValidationOutcomes(t *testing.T) {
	testCases := []struct {
		name       string
		code       vog.ValidationCode
		wantStatus int
		wantKey    string
	}{
		{"known not integral", vog.CodeKnownNotIntegral, http.StatusUnprocessableEntity, "known_not_integral"},
		{"unknown document", vog.CodeUnknown, http.StatusUnprocessableEntity, "unknown_document"},
		{"invalid signature", vog.CodeInvalidSignature, http.StatusUnprocessableEntity, "invalid_signature"},
		{"retry", vog.CodeRetry, http.StatusServiceUnavailable, "validation_unavailable"},
		{"provenance error", vog.CodeProvenanceError, http.StatusServiceUnavailable, "provenance_error"},
		{"signature server error", vog.CodeSignatureServerError, http.StatusServiceUnavailable, "signature_server_error"},
		{"provenance store error", vog.CodeProvenanceStoreError, http.StatusServiceUnavailable, "provenance_store_error"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			deps := defaultDeps()
			deps.validator.code = tc.code
			startTestServer(t, deps)

			resp, body, errResp := postFile[models.ErrorResponse](t, fmt.Sprintf(testHost, uploadEndpoint), "file", "vog.pdf", fakePdf)
			mustStatus(t, resp, tc.wantStatus, body)
			require.Equal(t, ErrorValidationFailed, errResp.Error)
			require.NotNil(t, errResp.Validation)
			require.Equal(t, int(tc.code), errResp.Validation.Code)
			require.Equal(t, tc.wantKey, errResp.Validation.Key)
			require.False(t, errResp.Validation.Authentic)
			require.Equal(t, tc.code.Retryable(), errResp.Validation.Retryable)
		})
	}
}

func TestUploadValidationServiceDown(t *testing.T) {
	deps := defaultDeps()
	deps.validator.err = errors.New("connection refused")
	startTestServer(t, deps)

	resp, body, errResp := postFile[models.ErrorResponse](t, fmt.Sprintf(testHost, uploadEndpoint), "file", "vog.pdf", fakePdf)
	mustStatus(t, resp, http.StatusServiceUnavailable, body)
	require.Equal(t, ErrorValidationService, errResp.Error)
}

func TestUploadNotAVog(t *testing.T) {
	deps := defaultDeps()
	deps.parser = fakeParser{err: fmt.Errorf("%w: title not found", vog.ErrNotAVog)}
	startTestServer(t, deps)

	resp, body, errResp := postFile[models.ErrorResponse](t, fmt.Sprintf(testHost, uploadEndpoint), "file", "vog.pdf", fakePdf)
	mustStatus(t, resp, http.StatusBadRequest, body)
	require.Equal(t, ErrorNotAVog, errResp.Error)
}

func TestStartDisclosure(t *testing.T) {
	deps := defaultDeps()
	startTestServer(t, deps)
	upload := uploadOk(t)

	resp, body, pkg := postJSON[models.DisclosureSessionResponse](t, fmt.Sprintf(testHost, disclosureEndpoint), models.SessionRequest{SessionId: upload.SessionId})
	mustStatus(t, resp, http.StatusOK, body)
	require.Equal(t, "https://irma.example/irma/session/xyz", pkg.SessionPtr.U)
	require.Equal(t, "disclosing", pkg.SessionPtr.Irmaqr)
	require.NotNil(t, pkg.FrontendRequest)
	// The requestor token stays server side.
	require.NotContains(t, string(body), `"token"`)
	require.Equal(t, "disclosure-jwt", deps.irma.startJwt)

	session, err := deps.storage.Retrieve(upload.SessionId)
	require.NoError(t, err)
	require.Equal(t, StageDisclosing, session.Stage)
	require.Equal(t, "tok", session.IrmaToken)
}

func TestStartDisclosureErrors(t *testing.T) {
	t.Run("unknown session", func(t *testing.T) {
		startTestServer(t, defaultDeps())
		resp, body, errResp := postJSON[models.ErrorResponse](t, fmt.Sprintf(testHost, disclosureEndpoint), models.SessionRequest{SessionId: "nope"})
		mustStatus(t, resp, http.StatusNotFound, body)
		require.Equal(t, ErrorUnknownSession, errResp.Error)
	})

	t.Run("missing session id", func(t *testing.T) {
		startTestServer(t, defaultDeps())
		resp, body, errResp := postJSON[models.ErrorResponse](t, fmt.Sprintf(testHost, disclosureEndpoint), map[string]string{})
		mustStatus(t, resp, http.StatusBadRequest, body)
		require.Equal(t, ErrorInvalidRequest, errResp.Error)
	})

	t.Run("irma server down", func(t *testing.T) {
		deps := defaultDeps()
		deps.irma.startErr = errors.New("irma down")
		startTestServer(t, deps)
		upload := uploadOk(t)
		resp, body, errResp := postJSON[models.ErrorResponse](t, fmt.Sprintf(testHost, disclosureEndpoint), models.SessionRequest{SessionId: upload.SessionId})
		mustStatus(t, resp, http.StatusBadGateway, body)
		require.Equal(t, ErrorIrmaServer, errResp.Error)
	})
}

func TestIssueHappyPath(t *testing.T) {
	deps := defaultDeps()
	startTestServer(t, deps)
	upload := uploadOk(t)
	startDisclosureOk(t, upload.SessionId)

	resp, body, issuance := postJSON[models.IssuanceResponse](t, fmt.Sprintf(testHost, issueEndpoint), models.SessionRequest{SessionId: upload.SessionId})
	mustStatus(t, resp, http.StatusOK, body)
	require.Equal(t, "issuance-jwt", issuance.Jwt)
	require.Equal(t, "https://irma.example", issuance.IrmaServerURL)
	require.True(t, issuance.Identity.Matched)
	require.Equal(t, SourcePassport, issuance.Identity.Source)

	require.Equal(t, irma.RequestorToken("tok"), deps.irma.asked)
	require.NotNil(t, deps.jwt.issuedDoc)
	require.Equal(t, "9999012026032500922", deps.jwt.issuedDoc.ReferenceNumber)
	require.Equal(t, SourcePassport, deps.jwt.issuedSource)

	// The session is consumed: a second issuance is not possible.
	_, err := deps.storage.Retrieve(upload.SessionId)
	require.Error(t, err)
	resp, body, errResp := postJSON[models.ErrorResponse](t, fmt.Sprintf(testHost, issueEndpoint), models.SessionRequest{SessionId: upload.SessionId})
	mustStatus(t, resp, http.StatusNotFound, body)
	require.Equal(t, ErrorUnknownSession, errResp.Error)
}

func TestIssueIdentityMismatchAllowsRetry(t *testing.T) {
	deps := defaultDeps()
	deps.irma.result = validDisclosure("PIET", "JANSEN", "1975-01-01")
	startTestServer(t, deps)
	upload := uploadOk(t)
	startDisclosureOk(t, upload.SessionId)

	resp, body, errResp := postJSON[models.ErrorResponse](t, fmt.Sprintf(testHost, issueEndpoint), models.SessionRequest{SessionId: upload.SessionId})
	mustStatus(t, resp, http.StatusForbidden, body)
	require.Equal(t, ErrorIdentityMismatch, errResp.Error)
	require.NotNil(t, errResp.Identity)
	require.False(t, errResp.Identity.Matched)
	require.False(t, errResp.Identity.DateOfBirthMatch)
	require.False(t, errResp.Identity.SurnameMatch)
	require.False(t, errResp.Identity.GivenNamesMatch)
	require.Equal(t, SourcePassport, errResp.Identity.Source)
	require.Nil(t, deps.jwt.issuedDoc, "nothing may be issued on a mismatch")

	// The VOG stays, the spent disclosure is forgotten...
	session, err := deps.storage.Retrieve(upload.SessionId)
	require.NoError(t, err)
	require.Equal(t, StageValidated, session.Stage)
	require.Empty(t, session.IrmaToken)

	// ...so issuing without a new disclosure is refused...
	resp, body, errResp = postJSON[models.ErrorResponse](t, fmt.Sprintf(testHost, issueEndpoint), models.SessionRequest{SessionId: upload.SessionId})
	mustStatus(t, resp, http.StatusConflict, body)
	require.Equal(t, ErrorDisclosureNotDone, errResp.Error)

	// ...and a new, matching disclosure succeeds.
	deps.irma.result = validDisclosure("Anna", "van der Berg", "03-02-1980")
	startDisclosureOk(t, upload.SessionId)
	resp, body, _ = postJSON[models.IssuanceResponse](t, fmt.Sprintf(testHost, issueEndpoint), models.SessionRequest{SessionId: upload.SessionId})
	mustStatus(t, resp, http.StatusOK, body)
}

func TestIssueDisclosureStates(t *testing.T) {
	t.Run("not finished", func(t *testing.T) {
		deps := defaultDeps()
		deps.irma.result.Status = irma.ServerStatusConnected
		startTestServer(t, deps)
		upload := uploadOk(t)
		startDisclosureOk(t, upload.SessionId)

		resp, body, errResp := postJSON[models.ErrorResponse](t, fmt.Sprintf(testHost, issueEndpoint), models.SessionRequest{SessionId: upload.SessionId})
		mustStatus(t, resp, http.StatusConflict, body)
		require.Equal(t, ErrorDisclosureNotDone, errResp.Error)

		// Still waiting: the disclosure session is kept for polling.
		session, err := deps.storage.Retrieve(upload.SessionId)
		require.NoError(t, err)
		require.Equal(t, StageDisclosing, session.Stage)
	})

	t.Run("invalid proof", func(t *testing.T) {
		deps := defaultDeps()
		deps.irma.result.ProofStatus = irma.ProofStatusInvalid
		startTestServer(t, deps)
		upload := uploadOk(t)
		startDisclosureOk(t, upload.SessionId)

		resp, body, errResp := postJSON[models.ErrorResponse](t, fmt.Sprintf(testHost, issueEndpoint), models.SessionRequest{SessionId: upload.SessionId})
		mustStatus(t, resp, http.StatusForbidden, body)
		require.Equal(t, ErrorDisclosureInvalid, errResp.Error)
		require.Nil(t, deps.jwt.issuedDoc)
	})

	t.Run("irma server down", func(t *testing.T) {
		deps := defaultDeps()
		deps.irma.resultErr = errors.New("irma down")
		startTestServer(t, deps)
		upload := uploadOk(t)
		startDisclosureOk(t, upload.SessionId)

		resp, body, errResp := postJSON[models.ErrorResponse](t, fmt.Sprintf(testHost, issueEndpoint), models.SessionRequest{SessionId: upload.SessionId})
		mustStatus(t, resp, http.StatusBadGateway, body)
		require.Equal(t, ErrorIrmaServer, errResp.Error)
	})

	t.Run("disclosure never started", func(t *testing.T) {
		deps := defaultDeps()
		startTestServer(t, deps)
		upload := uploadOk(t)

		resp, body, errResp := postJSON[models.ErrorResponse](t, fmt.Sprintf(testHost, issueEndpoint), models.SessionRequest{SessionId: upload.SessionId})
		mustStatus(t, resp, http.StatusConflict, body)
		require.Equal(t, ErrorDisclosureNotDone, errResp.Error)
	})
}

func TestIssueWithBrpDisclosure(t *testing.T) {
	deps := defaultDeps()
	deps.irma.result.Disclosed = [][]*irma.DisclosedAttribute{{
		disclosedAttr(testIdentityCredentials.Brp+"."+BrpAttrFirstNames, "Anna Maria"),
		disclosedAttr(testIdentityCredentials.Brp+"."+BrpAttrPrefix, "van der"),
		disclosedAttr(testIdentityCredentials.Brp+"."+BrpAttrFamilyName, "Berg"),
		disclosedAttr(testIdentityCredentials.Brp+"."+BrpAttrDateOfBirth, "03-02-1980"),
	}}
	startTestServer(t, deps)
	upload := uploadOk(t)
	startDisclosureOk(t, upload.SessionId)

	resp, body, issuance := postJSON[models.IssuanceResponse](t, fmt.Sprintf(testHost, issueEndpoint), models.SessionRequest{SessionId: upload.SessionId})
	mustStatus(t, resp, http.StatusOK, body)
	require.Equal(t, SourceBrp, issuance.Identity.Source)
	require.Equal(t, SourceBrp, deps.jwt.issuedSource)
}
