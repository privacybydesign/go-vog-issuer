package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sync"
	"testing"
	"time"

	"go-vog-issuer/vog"

	"github.com/privacybydesign/irmago/irma"
	"github.com/privacybydesign/irmago/irma/server"
	"github.com/stretchr/testify/require"
)

var testConfig = ServerConfig{
	Host: "localhost",
	Port: 8081,
}

const testHost = "http://localhost:8081%s"

// fakeValidator answers with a fixed code (or error) and records what it saw.
type fakeValidator struct {
	code  vog.ValidationCode
	err   error
	calls int
	last  []byte
	mutex sync.Mutex
}

func (f *fakeValidator) Validate(_ context.Context, pdf []byte, _ string) (vog.ValidationCode, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.calls++
	f.last = pdf
	return f.code, f.err
}

// fakeParser returns a fixed document (or error).
type fakeParser struct {
	doc *vog.Document
	err error
}

func (f fakeParser) Parse(_ []byte) (*vog.Document, error) {
	if f.err != nil {
		return nil, f.err
	}
	copied := *f.doc
	return &copied, nil
}

// fakeJwtCreator returns fixed JWTs and records the issued document.
type fakeJwtCreator struct {
	disclosureJwt string
	issuanceJwt   string
	issuedDoc     *vog.Document
	issuedSource  string
	err           error
}

func (f *fakeJwtCreator) CreateDisclosureJwt() (string, error) {
	return f.disclosureJwt, f.err
}

func (f *fakeJwtCreator) CreateIssuanceJwt(doc *vog.Document, source string) (string, error) {
	f.issuedDoc = doc
	f.issuedSource = source
	return f.issuanceJwt, f.err
}

// fakeIrmaClient simulates the IRMA server: StartSession hands out a token and
// GetSessionResult returns the configured result for that token.
type fakeIrmaClient struct {
	startErr  error
	resultErr error
	result    *server.SessionResult
	token     irma.RequestorToken
	startJwt  string
	asked     irma.RequestorToken
}

func (f *fakeIrmaClient) StartSession(_ context.Context, signedJwt string) (*server.SessionPackage, error) {
	f.startJwt = signedJwt
	if f.startErr != nil {
		return nil, f.startErr
	}
	return &server.SessionPackage{
		SessionPtr:      &irma.Qr{URL: "https://irma.example/irma/session/xyz", Type: irma.ActionDisclosing},
		Token:           f.token,
		FrontendRequest: &irma.FrontendSessionRequest{Authorization: "auth", MinProtocolVersion: irma.NewVersion(1, 0), MaxProtocolVersion: irma.NewVersion(1, 1)},
	}, nil
}

func (f *fakeIrmaClient) GetSessionResult(_ context.Context, token irma.RequestorToken) (*server.SessionResult, error) {
	f.asked = token
	if f.resultErr != nil {
		return nil, f.resultErr
	}
	return f.result, nil
}

// validDisclosure builds a DONE/VALID passport disclosure for the given person.
func validDisclosure(givenNames, lastName, dateOfBirth string) *server.SessionResult {
	return &server.SessionResult{
		Token:       "tok",
		Status:      irma.ServerStatusDone,
		Type:        irma.ActionDisclosing,
		ProofStatus: irma.ProofStatusValid,
		Disclosed: [][]*irma.DisclosedAttribute{{
			disclosedAttr(testIdentityCredentials.Passport+"."+DocAttrFirstName, givenNames),
			disclosedAttr(testIdentityCredentials.Passport+"."+DocAttrLastName, lastName),
			disclosedAttr(testIdentityCredentials.Passport+"."+DocAttrDateOfBirth, dateOfBirth),
		}},
	}
}

type testDeps struct {
	storage   SessionStorage
	validator *fakeValidator
	parser    fakeParser
	jwt       *fakeJwtCreator
	irma      *fakeIrmaClient
}

func defaultDeps() *testDeps {
	return &testDeps{
		storage:   NewInMemorySessionStorage(),
		validator: &fakeValidator{code: vog.CodeAuthentic},
		parser:    fakeParser{doc: testVogDocument()},
		jwt:       &fakeJwtCreator{disclosureJwt: "disclosure-jwt", issuanceJwt: "issuance-jwt"},
		irma:      &fakeIrmaClient{token: "tok", result: validDisclosure("ANNA MARIA", "VAN DER BERG", "1980-02-03")},
	}
}

func startTestServer(t *testing.T, deps *testDeps) *Server {
	t.Helper()

	state := &ServerState{
		irmaServerURL:       "https://irma.example",
		sessionStorage:      deps.storage,
		jwtCreator:          deps.jwt,
		validator:           deps.validator,
		parser:              deps.parser,
		irmaClient:          deps.irma,
		identityCredentials: testIdentityCredentials,
		maxUploadSize:       1 << 20,
	}

	srv, err := NewServer(state, testConfig)
	require.NoError(t, err)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("server error: %v", err)
		}
	}()

	waitUntilHealthy(t, fmt.Sprintf(testHost, "/api/health"))
	t.Cleanup(func() {
		if err := srv.Stop(); err != nil {
			t.Logf("error shutting down server: %v", err)
		}
	})
	return srv
}

func waitUntilHealthy(t *testing.T, url string) {
	t.Helper()
	const maxAttempts = 50
	for i := 0; i < maxAttempts; i++ {
		if resp, err := http.Get(url); err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server did not start in time")
}

func postJSON[T any](t *testing.T, url string, payload any) (*http.Response, []byte, *T) {
	t.Helper()

	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		require.NoError(t, err)
		body = bytes.NewBuffer(b)
	}
	resp, err := http.Post(url, "application/json", body)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var v T
	_ = json.Unmarshal(respBody, &v)
	return resp, respBody, &v
}

// postFile uploads content as multipart field "file".
func postFile[T any](t *testing.T, url, field, filename string, content []byte) (*http.Response, []byte, *T) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if field != "" {
		part, err := writer.CreateFormFile(field, filename)
		require.NoError(t, err)
		_, err = part.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	resp, err := http.Post(url, writer.FormDataContentType(), &body)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var v T
	_ = json.Unmarshal(respBody, &v)
	return resp, respBody, &v
}

func mustStatus(t *testing.T, resp *http.Response, want int, body []byte) {
	t.Helper()
	require.Equalf(t, want, resp.StatusCode, "body: %s", body)
}
