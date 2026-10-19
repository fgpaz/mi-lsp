package embed

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbedQueryErrorIsTypedAndSanitized(t *testing.T) {
	const privateBody = `{"error":"private-provider-detail"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(privateBody))
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, Model: "fixture-model"})
	_, err := client.EmbedQuery(context.Background(), "private-query")
	var typed *QueryError
	if !errors.As(err, &typed) {
		t.Fatalf("EmbedQuery error = %T; want *QueryError", err)
	}
	if typed.Class != "http" || typed.Subphase != "response" || typed.Operation != "embed_query" || typed.HTTPStatus != http.StatusUnauthorized || typed.Code != "SEM_EMBED_HTTP_ERROR" {
		t.Fatalf("unexpected typed error: %+v", typed)
	}
	if strings.Contains(err.Error(), privateBody) || strings.Contains(err.Error(), "private-query") || strings.Contains(err.Error(), server.URL) {
		t.Fatalf("error exposed private data: %q", err)
	}
}
