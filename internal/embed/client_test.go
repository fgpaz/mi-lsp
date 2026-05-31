package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbedWithHttptestServer(t *testing.T) {
	// Create a fake embeddings server that returns deterministic vectors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req requestPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		// Return a deterministic embedding for each input text
		data := make([]responseData, len(req.Input))
		for i := range req.Input {
			// Simple deterministic embedding: hash-like values based on text
			embedding := make([]float32, 4)
			for j := range embedding {
				// Use input length and text chars to create deterministic values
				sum := float32(len(req.Input[i])) * float32(j+1)
				for _, ch := range req.Input[i] {
					sum += float32(ch)
				}
				// Use int conversion for modulo
				embedding[j] = float32(int(sum)%10) / 10.0
			}
			data[i].Embedding = embedding
		}

		resp := responsePayload{Data: data}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Config{
		Provider:  "openai-compatible",
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKeyEnv: "",
		Dim:       4,
		BatchSize: 32,
		TimeoutMS: 5000,
	}

	client := New(cfg)
	texts := []string{"hello", "world"}
	embeddings, err := client.Embed(context.Background(), texts)

	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}

	for i, emb := range embeddings {
		if len(emb) != 4 {
			t.Errorf("embedding %d has wrong dimension: got %d, expected 4", i, len(emb))
		}
	}
}

func TestAuthorizationHeaderPresent(t *testing.T) {
	authHeaderSeen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" && authHeader == "Bearer test-key-123" {
			authHeaderSeen = true
		}

		resp := responsePayload{
			Data: []responseData{
				{Embedding: []float32{0.1, 0.2, 0.3, 0.4}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Config{
		Provider:  "openai-compatible",
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKeyEnv: "TEST_API_KEY",
		Dim:       4,
		BatchSize: 32,
		TimeoutMS: 5000,
	}

	// Set the API key in the environment
	t.Setenv("TEST_API_KEY", "test-key-123")

	client := New(cfg)
	_, err := client.Embed(context.Background(), []string{"test"})

	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if !authHeaderSeen {
		t.Error("Authorization header was not set")
	}
}

func TestAuthorizationHeaderAbsent(t *testing.T) {
	authHeaderPresent := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			authHeaderPresent = true
		}

		resp := responsePayload{
			Data: []responseData{
				{Embedding: []float32{0.1, 0.2, 0.3, 0.4}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Config{
		Provider:  "openai-compatible",
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKeyEnv: "TEST_API_KEY_EMPTY",
		Dim:       4,
		BatchSize: 32,
		TimeoutMS: 5000,
	}

	// Don't set the env var, so it's empty
	t.Setenv("TEST_API_KEY_EMPTY", "")

	client := New(cfg)
	_, err := client.Embed(context.Background(), []string{"test"})

	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if authHeaderPresent {
		t.Error("Authorization header should not be set when env is empty")
	}
}

func TestBatching(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		var req requestPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		// Return embeddings for all inputs in this batch
		data := make([]responseData, len(req.Input))
		for i := range req.Input {
			data[i].Embedding = []float32{0.1, 0.2, 0.3, 0.4}
		}

		resp := responsePayload{Data: data}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Config{
		Provider:  "openai-compatible",
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKeyEnv: "",
		Dim:       4,
		BatchSize: 3, // Batch size of 3
		TimeoutMS: 5000,
	}

	client := New(cfg)
	// Send 10 texts with batch size 3: should result in 4 requests (3, 3, 3, 1)
	texts := make([]string, 10)
	for i := range texts {
		texts[i] = "text"
	}

	embeddings, err := client.Embed(context.Background(), texts)

	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if len(embeddings) != 10 {
		t.Fatalf("expected 10 embeddings, got %d", len(embeddings))
	}

	expectedRequests := 4
	if requestCount != expectedRequests {
		t.Errorf("expected %d requests, got %d", expectedRequests, requestCount)
	}
}

func TestEmptyInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called for empty input")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := Config{
		Provider:  "openai-compatible",
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKeyEnv: "",
		Dim:       4,
		BatchSize: 32,
		TimeoutMS: 5000,
	}

	client := New(cfg)
	embeddings, err := client.Embed(context.Background(), []string{})

	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if len(embeddings) != 0 {
		t.Errorf("expected 0 embeddings, got %d", len(embeddings))
	}
}

func TestEmbedOneConvenience(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := responsePayload{
			Data: []responseData{
				{Embedding: []float32{0.1, 0.2, 0.3, 0.4}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Config{
		Provider:  "openai-compatible",
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKeyEnv: "",
		Dim:       4,
		BatchSize: 32,
		TimeoutMS: 5000,
	}

	client := New(cfg)
	embedding, err := client.EmbedOne(context.Background(), "test text")

	if err != nil {
		t.Fatalf("EmbedOne failed: %v", err)
	}

	if len(embedding) != 4 {
		t.Errorf("expected 4-dim embedding, got %d", len(embedding))
	}
}

func TestResponseFewerEmbeddingsThanInputs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return fewer embeddings than requested
		resp := responsePayload{
			Data: []responseData{
				{Embedding: []float32{0.1, 0.2}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Config{
		Provider:  "openai-compatible",
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKeyEnv: "",
		Dim:       2,
		BatchSize: 32,
		TimeoutMS: 5000,
	}

	client := New(cfg)
	_, err := client.Embed(context.Background(), []string{"text1", "text2"})

	if err == nil {
		t.Fatal("expected error for fewer embeddings than inputs")
	}
}

func TestNonSuccessStatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := Config{
		Provider:  "openai-compatible",
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKeyEnv: "",
		Dim:       4,
		BatchSize: 32,
		TimeoutMS: 5000,
	}

	client := New(cfg)
	_, err := client.Embed(context.Background(), []string{"test"})

	if err == nil {
		t.Fatal("expected error for non-2xx status code")
	}
}

func TestDefaultBatchSize(t *testing.T) {
	cfg := Config{
		Provider:  "openai-compatible",
		BaseURL:   "http://localhost:8080",
		Model:     "test-model",
		APIKeyEnv: "",
		Dim:       4,
		BatchSize: 0, // Should default to 32
		TimeoutMS: 5000,
	}

	client := New(cfg)

	if client.cfg.BatchSize != 32 {
		t.Errorf("expected BatchSize to default to 32, got %d", client.cfg.BatchSize)
	}
}

func TestDefaultTimeoutMS(t *testing.T) {
	cfg := Config{
		Provider:  "openai-compatible",
		BaseURL:   "http://localhost:8080",
		Model:     "test-model",
		APIKeyEnv: "",
		Dim:       4,
		BatchSize: 10,
		TimeoutMS: 0, // Should default to 30000
	}

	client := New(cfg)

	if client.cfg.TimeoutMS != 30000 {
		t.Errorf("expected TimeoutMS to default to 30000, got %d", client.cfg.TimeoutMS)
	}
}

func TestAuthorizationHeaderNotSetWhenAPIKeyEnvEmpty(t *testing.T) {
	authHeaderPresent := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			authHeaderPresent = true
		}

		resp := responsePayload{
			Data: []responseData{
				{Embedding: []float32{0.1, 0.2, 0.3, 0.4}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Config{
		Provider:  "openai-compatible",
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKeyEnv: "", // Empty APIKeyEnv
		Dim:       4,
		BatchSize: 32,
		TimeoutMS: 5000,
	}

	client := New(cfg)
	_, err := client.Embed(context.Background(), []string{"test"})

	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if authHeaderPresent {
		t.Error("Authorization header should not be set when APIKeyEnv is empty")
	}
}
