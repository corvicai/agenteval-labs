package orchestrator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noopParser(_ map[string]any) (string, error) { return "", nil }

// A non-JSON upstream error body (a proxy / gateway HTML page — the typical
// shape when Cloud Run egress is misconfigured) must be preserved in the error
// instead of being swallowed by the JSON decode and replaced with a bare status.
func TestCallOpenAI_PreservesNonJSONErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body>502 Bad Gateway from proxy</body></html>"))
	}))
	defer srv.Close()

	_, _, err := callOpenAIWithBaseURLAndHeaders(
		context.Background(), "sk-x", "", srv.URL, "chat/completions",
		map[string]any{"model": "gpt-4o-mini"}, noopParser, nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "502")
	assert.Contains(t, err.Error(), "Bad Gateway from proxy")
}

// A structured OpenAI JSON error must still surface its message (regression).
func TestCallOpenAI_PreservesJSONErrorMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided"}}`))
	}))
	defer srv.Close()

	_, _, err := callOpenAIWithBaseURLAndHeaders(
		context.Background(), "sk-x", "", srv.URL, "chat/completions",
		map[string]any{}, noopParser, nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Incorrect API key provided")
}
