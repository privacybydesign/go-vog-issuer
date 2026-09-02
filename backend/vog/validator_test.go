package vog

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidationCodeSemantics(t *testing.T) {
	require.True(t, CodeAuthentic.Authentic())
	require.False(t, CodeAuthentic.DocumentRejected())
	require.False(t, CodeAuthentic.Retryable())
	require.Equal(t, "authentic", CodeAuthentic.Key())

	for _, code := range []ValidationCode{CodeKnownNotIntegral, CodeUnknown, CodeInvalidSignature} {
		require.False(t, code.Authentic(), code)
		require.True(t, code.DocumentRejected(), code)
		require.False(t, code.Retryable(), code)
	}
	for _, code := range []ValidationCode{CodeRetry, CodeProvenanceError, CodeSignatureServerError, CodeProvenanceStoreError} {
		require.False(t, code.Authentic(), code)
		require.False(t, code.DocumentRejected(), code)
		require.True(t, code.Retryable(), code)
	}

	keys := map[string]bool{}
	for code := CodeAuthentic; code <= CodeProvenanceStoreError; code++ {
		require.False(t, keys[code.Key()], "duplicate key %s", code.Key())
		keys[code.Key()] = true
		require.NotEmpty(t, code.Description())
	}
	require.Equal(t, "unknown_response_code", ValidationCode(42).Key())
	require.Contains(t, ValidationCode(42).Description(), "42")
}

func TestGaavClientValidate(t *testing.T) {
	var gotContentType, gotFilename, gotPartType string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		gotContentType = r.Header.Get("Content-Type")
		file, header, err := r.FormFile("file")
		require.NoError(t, err)
		defer func() { _ = file.Close() }()
		gotFilename = header.Filename
		gotPartType = header.Header.Get("Content-Type")
		gotBody, err = io.ReadAll(file)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response_code":2}`))
	}))
	defer server.Close()

	client := NewGaavClient(server.URL, time.Second)
	code, err := client.Validate(context.Background(), []byte("%PDF-1.5 fake"), "vog.pdf")
	require.NoError(t, err)
	require.Equal(t, CodeUnknown, code)
	require.Contains(t, gotContentType, "multipart/form-data")
	require.Equal(t, "vog.pdf", gotFilename)
	// validatie.nl answers "unknown document" unless the part is declared as a PDF.
	require.Equal(t, "application/pdf", gotPartType)
	require.Equal(t, []byte("%PDF-1.5 fake"), gotBody)
}

func TestGaavClientErrors(t *testing.T) {
	testCases := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"server error", http.StatusInternalServerError, "boom", "status 500"},
		{"method not allowed", http.StatusMethodNotAllowed, `{"detail":"Method \"GET\" not allowed."}`, "status 405"},
		{"not json", http.StatusOK, "<html>", "decode"},
		{"missing code", http.StatusOK, `{"foo":1}`, "lacks response_code"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			client := NewGaavClient(server.URL, time.Second)
			_, err := client.Validate(context.Background(), []byte("%PDF"), "")
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestGaavClientUnreachable(t *testing.T) {
	client := NewGaavClient("http://127.0.0.1:1", time.Second)
	_, err := client.Validate(context.Background(), []byte("%PDF"), "")
	require.Error(t, err)
}

func TestGaavClientDefaults(t *testing.T) {
	client := NewGaavClient("", 0)
	require.Equal(t, DefaultValidationURL, client.url)
	require.Equal(t, 30*time.Second, client.client.Timeout)
}
