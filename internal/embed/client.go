package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config holds the configuration for the embeddings client.
type Config struct {
	Provider       string
	BaseURL        string
	Model          string
	APIKeyEnv      string
	Dim            int
	BatchSize      int
	TimeoutMS      int
	EncodingFormat string
	UserAgent      string
}

// Client is an OpenAI-compatible embeddings client.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// New creates a new embeddings client with the given config.
// Defaults: BatchSize=32 (if ≤0), TimeoutMS=30000 (if ≤0).
func New(cfg Config) *Client {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 32
	}
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = 30000
	}
	if strings.TrimSpace(cfg.EncodingFormat) == "" {
		cfg.EncodingFormat = "float"
	}
	if strings.TrimSpace(cfg.UserAgent) == "" {
		cfg.UserAgent = "OpenAI/Go mi-lsp-embeddings/1.0"
	}

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.TimeoutMS) * time.Millisecond,
		},
	}
}

// requestPayload is the JSON request sent to the embeddings endpoint.
type requestPayload struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format,omitempty"`
}

// responseData represents a single embedding in the response.
type responseData struct {
	Embedding []float32 `json:"embedding"`
}

// responsePayload is the JSON response from the embeddings endpoint.
type responsePayload struct {
	Data []responseData `json:"data"`
}

// Embed batches texts and sends them to the embeddings endpoint.
// Returns a slice of embeddings, one per input text, preserving order.
// Returns an error if the response has fewer embeddings than inputs,
// or if the HTTP request/response fails.
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	var results [][]float32

	// Process texts in batches
	for i := 0; i < len(texts); i += c.cfg.BatchSize {
		end := i + c.cfg.BatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		embeddings, err := c.embedBatch(ctx, batch)
		if err != nil {
			return nil, err
		}

		results = append(results, embeddings...)
	}

	return results, nil
}

// embedBatch sends a single batch to the endpoint and returns the embeddings.
func (c *Client) embedBatch(ctx context.Context, batch []string) ([][]float32, error) {
	payload := requestPayload{
		Model:          c.cfg.Model,
		Input:          batch,
		EncodingFormat: c.cfg.EncodingFormat,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	// Set Authorization header if API key is available
	if c.cfg.APIKeyEnv != "" {
		apiKey := os.Getenv(c.cfg.APIKeyEnv)
		if apiKey != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("embeddings endpoint returned %d: %s", resp.StatusCode, string(respBody))
	}

	var respPayload responsePayload
	if err := json.NewDecoder(resp.Body).Decode(&respPayload); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(respPayload.Data) < len(batch) {
		return nil, fmt.Errorf("response has %d embeddings but expected %d", len(respPayload.Data), len(batch))
	}

	result := make([][]float32, len(batch))
	for i := range batch {
		if c.cfg.Dim > 0 && len(respPayload.Data[i].Embedding) != c.cfg.Dim {
			return nil, fmt.Errorf("embedding %d has dimension %d but expected %d", i, len(respPayload.Data[i].Embedding), c.cfg.Dim)
		}
		result[i] = respPayload.Data[i].Embedding
	}

	return result, nil
}

// EmbedOne is a convenience method that embeds a single text.
func (c *Client) EmbedOne(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := c.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return []float32{}, nil
	}
	return embeddings[0], nil
}
