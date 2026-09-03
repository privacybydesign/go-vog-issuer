package vog

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// noRetry makes a client answer from its first attempt so tests that are not
// about retrying stay fast and deterministic.
var noRetry = RetryPolicy{MaxAttempts: 1}

// fastRetry retries with a negligible pause.
func fastRetry(attempts int) RetryPolicy {
	return RetryPolicy{MaxAttempts: attempts, Delay: time.Millisecond}
}

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

	client := NewGaavClient(server.URL, time.Second, noRetry)
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
		name      string
		status    int
		body      string
		wantErr   string
		transient bool
	}{
		{"server error", http.StatusInternalServerError, "boom", "status 500", true},
		{"bad gateway", http.StatusBadGateway, "<html>502</html>", "status 502", true},
		{"service unavailable", http.StatusServiceUnavailable, "", "status 503", true},
		{"method not allowed", http.StatusMethodNotAllowed, `{"detail":"Method \"GET\" not allowed."}`, "status 405", false},
		{"not json", http.StatusOK, "<html>", "decode", false},
		{"missing code", http.StatusOK, `{"foo":1}`, "lacks response_code", false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			client := NewGaavClient(server.URL, time.Second, noRetry)
			_, err := client.Validate(context.Background(), []byte("%PDF"), "")
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
			require.Equal(t, tc.transient, Transient(err))
		})
	}
}

func TestGaavClientUnreachable(t *testing.T) {
	client := NewGaavClient("http://127.0.0.1:1", time.Second, noRetry)
	_, err := client.Validate(context.Background(), []byte("%PDF"), "")
	require.Error(t, err)
	require.True(t, Transient(err), err)
}

func TestGaavClientDefaults(t *testing.T) {
	client := NewGaavClient("", 0, RetryPolicy{})
	require.Equal(t, DefaultValidationURL, client.url)
	require.Equal(t, DefaultTimeout, client.Timeout())
	require.Equal(t, RetryPolicy{MaxAttempts: DefaultMaxAttempts, Delay: DefaultRetryDelay}, client.Retry())

	custom := NewGaavClient("", 5*time.Second, RetryPolicy{MaxAttempts: 5, Delay: 2 * time.Second})
	require.Equal(t, 5*time.Second, custom.Timeout())
	require.Equal(t, RetryPolicy{MaxAttempts: 5, Delay: 2 * time.Second}, custom.Retry())
}

// scriptedServer answers consecutive requests with the given responses and
// keeps answering the last one afterwards.
func scriptedServer(t *testing.T, responses ...func(w http.ResponseWriter)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Every attempt must carry the full document again.
		file, _, err := r.FormFile("file")
		require.NoError(t, err)
		body, err := io.ReadAll(file)
		require.NoError(t, err)
		_ = file.Close()
		require.Equal(t, "%PDF-1.7 doc", string(body))

		n := int(calls.Add(1)) - 1
		if n >= len(responses) {
			n = len(responses) - 1
		}
		responses[n](w)
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func answerStatus(status int) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte("upstream trouble"))
	}
}

func answerCode(code ValidationCode) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response_code":` + string(rune('0'+int(code))) + `}`))
	}
}

func TestGaavClientRetriesServerErrors(t *testing.T) {
	server, calls := scriptedServer(t, answerStatus(503), answerStatus(502), answerCode(CodeAuthentic))

	client := NewGaavClient(server.URL, time.Second, fastRetry(3))
	code, err := client.Validate(context.Background(), []byte("%PDF-1.7 doc"), "vog.pdf")
	require.NoError(t, err)
	require.Equal(t, CodeAuthentic, code)
	require.Equal(t, int32(3), calls.Load())
}

func TestGaavClientRetriesRetryableCodes(t *testing.T) {
	for _, retryable := range []ValidationCode{CodeRetry, CodeProvenanceError, CodeSignatureServerError, CodeProvenanceStoreError} {
		t.Run(retryable.Key(), func(t *testing.T) {
			server, calls := scriptedServer(t, answerCode(retryable), answerCode(CodeAuthentic))

			client := NewGaavClient(server.URL, time.Second, fastRetry(3))
			code, err := client.Validate(context.Background(), []byte("%PDF-1.7 doc"), "vog.pdf")
			require.NoError(t, err)
			require.Equal(t, CodeAuthentic, code)
			require.Equal(t, int32(2), calls.Load())
		})
	}
}

func TestGaavClientReturnsLastRetryableCodeWhenExhausted(t *testing.T) {
	server, calls := scriptedServer(t, answerCode(CodeRetry))

	client := NewGaavClient(server.URL, time.Second, fastRetry(3))
	code, err := client.Validate(context.Background(), []byte("%PDF-1.7 doc"), "vog.pdf")
	require.NoError(t, err)
	require.Equal(t, CodeRetry, code)
	require.Equal(t, int32(3), calls.Load())
}

func TestGaavClientReturnsLastErrorWhenExhausted(t *testing.T) {
	server, calls := scriptedServer(t, answerStatus(503))

	client := NewGaavClient(server.URL, time.Second, fastRetry(4))
	_, err := client.Validate(context.Background(), []byte("%PDF-1.7 doc"), "vog.pdf")
	require.Error(t, err)
	require.Contains(t, err.Error(), "status 503")
	require.True(t, Transient(err))
	require.Equal(t, int32(4), calls.Load())
}

func TestGaavClientDoesNotRetryFinalOutcomes(t *testing.T) {
	testCases := []struct {
		name   string
		answer func(http.ResponseWriter)
	}{
		{"authentic", answerCode(CodeAuthentic)},
		{"rejected", answerCode(CodeUnknown)},
		{"tampered", answerCode(CodeKnownNotIntegral)},
		{"invalid signature", answerCode(CodeInvalidSignature)},
		{"client error", answerStatus(http.StatusBadRequest)},
		{"garbage", func(w http.ResponseWriter) { _, _ = w.Write([]byte("<html>")) }},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server, calls := scriptedServer(t, tc.answer, answerCode(CodeAuthentic))

			client := NewGaavClient(server.URL, time.Second, fastRetry(3))
			_, _ = client.Validate(context.Background(), []byte("%PDF-1.7 doc"), "vog.pdf")
			require.Equal(t, int32(1), calls.Load(), "a final answer must not be retried")
		})
	}
}

func TestGaavClientDoesNotRetryTimeouts(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer server.Close()

	client := NewGaavClient(server.URL, 50*time.Millisecond, fastRetry(3))
	_, err := client.Validate(context.Background(), []byte("%PDF"), "")
	require.Error(t, err)
	require.False(t, Transient(err), err)
	require.Equal(t, int32(1), calls.Load())
}

func TestGaavClientStopsRetryingWhenContextEnds(t *testing.T) {
	server, calls := scriptedServer(t, answerStatus(503))

	ctx, cancel := context.WithCancel(context.Background())
	client := NewGaavClient(server.URL, time.Second, RetryPolicy{MaxAttempts: 5, Delay: time.Hour})
	done := make(chan error, 1)
	go func() {
		_, err := client.Validate(ctx, []byte("%PDF-1.7 doc"), "vog.pdf")
		done <- err
	}()

	// The first attempt fails and the client starts its long pause; cancelling
	// must end the wait and hand back that failure.
	require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second, 5*time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.Error(t, err)
		require.Contains(t, err.Error(), "status 503")
	case <-time.After(time.Second):
		t.Fatal("Validate did not return after the context was cancelled")
	}
	require.Equal(t, int32(1), calls.Load())
}

func TestTransientOnlyMatchesMarkedErrors(t *testing.T) {
	require.False(t, Transient(nil))
	require.False(t, Transient(errors.New("plain")))
	require.True(t, Transient(&transientError{errors.New("wrapped")}))
}
