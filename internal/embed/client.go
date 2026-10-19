package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config holds the configuration for the embeddings client.
type Config struct {
	Provider       string
	BaseURL        string
	BaseURLs       []string
	Model          string
	APIKeyEnv      string
	APIKey         string
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

// APIKeyMissingError reports that a configured API key environment variable has
// no usable value. Its public text is intentionally stable and secret-free.
type APIKeyMissingError struct{}

func (*APIKeyMissingError) Error() string { return "SEM_API_KEY_MISSING" }

// QueryError describes an embedding failure without retaining provider text,
// request data, URLs, headers, or credentials.
type QueryError struct {
	Class      string `json:"class"`
	Subphase   string `json:"subphase"`
	Operation  string `json:"operation"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Code       string `json:"code"`
}

func (e *QueryError) Error() string { return e.Code }

func queryOperation(priority string) string {
	switch priority {
	case "alta":
		return "embed_query"
	case "baja":
		return "embed_index"
	default:
		return "embed"
	}
}

func queryError(priority, class, subphase, code string, status int) *QueryError {
	return &QueryError{Class: class, Subphase: subphase, Operation: queryOperation(priority), HTTPStatus: status, Code: code}
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
	return c.embed(ctx, texts, "")
}

// EmbedIndex batches index embeddings at low gateway priority.
func (c *Client) EmbedIndex(ctx context.Context, texts []string) ([][]float32, error) {
	return c.embed(ctx, texts, "baja")
}

// EmbedQuery embeds an interactive search query at high gateway priority.
func (c *Client) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := c.embed(ctx, []string{text}, "alta")
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return []float32{}, nil
	}
	return embeddings[0], nil
}

func (c *Client) embed(ctx context.Context, texts []string, priority string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	var results [][]float32

	// Process texts in batches
	for i := 0; i < len(texts); i += c.cfg.BatchSize {
		end := min(i+c.cfg.BatchSize, len(texts))
		batch := texts[i:end]

		embeddings, err := c.embedBatch(ctx, batch, priority)
		if err != nil {
			return nil, err
		}

		results = append(results, embeddings...)
	}

	return results, nil
}

// embedBatch sends a single batch to the endpoint and returns the embeddings.
func (c *Client) embedBatch(ctx context.Context, batch []string, priority string) ([][]float32, error) {
	apiKeyEnv := strings.TrimSpace(c.cfg.APIKeyEnv)
	apiKey := c.cfg.APIKey
	if apiKeyEnv != "" {
		apiKey = os.Getenv(apiKeyEnv)
		if strings.TrimSpace(apiKey) == "" {
			return nil, &APIKeyMissingError{}
		}
	}

	payload := requestPayload{
		Model:          c.cfg.Model,
		Input:          batch,
		EncodingFormat: c.cfg.EncodingFormat,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, queryError(priority, "request", "encode", "SEM_EMBED_REQUEST_INVALID", 0)
	}

	baseURLs := append([]string(nil), c.cfg.BaseURLs...)
	if len(baseURLs) == 0 && strings.TrimSpace(c.cfg.BaseURL) != "" {
		baseURLs = []string{c.cfg.BaseURL}
	}
	if len(baseURLs) == 0 {
		return nil, queryError(priority, "configuration", "resolve_endpoint", "SEM_EMBED_ENDPOINT_UNCONFIGURED", 0)
	}
	var lastErr error
	for _, baseURL := range baseURLs {
		result, retry, err := c.embedAt(ctx, strings.TrimRight(baseURL, "/")+"/embeddings", body, apiKeyEnv, apiKey, batch, priority)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) embedAt(ctx context.Context, url string, body []byte, apiKeyEnv, apiKey string, batch []string, priority string) ([][]float32, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, false, queryError(priority, "request", "prepare", "SEM_EMBED_REQUEST_INVALID", 0)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	if priority != "" {
		req.Header.Set("X-Micro-Proxy-Priority", priority)
	}

	// Set Authorization only when APIKeyEnv names a non-empty environment value.
	if apiKeyEnv != "" || apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, queryError(priority, "transport", "send", "SEM_EMBED_TRANSPORT_FAILED", 0)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode >= 500, queryError(priority, "http", "response", "SEM_EMBED_HTTP_ERROR", resp.StatusCode)
	}

	var respPayload responsePayload
	if err := json.NewDecoder(resp.Body).Decode(&respPayload); err != nil {
		return nil, false, queryError(priority, "response", "decode", "SEM_EMBED_RESPONSE_INVALID", resp.StatusCode)
	}

	if len(respPayload.Data) < len(batch) {
		return nil, false, queryError(priority, "response", "shape", "SEM_EMBED_RESPONSE_INCOMPLETE", resp.StatusCode)
	}

	result := make([][]float32, len(batch))
	for i := range batch {
		if c.cfg.Dim > 0 && len(respPayload.Data[i].Embedding) != c.cfg.Dim {
			return nil, false, queryError(priority, "response", "shape", "SEM_EMBED_DIMENSION_MISMATCH", resp.StatusCode)
		}
		result[i] = respPayload.Data[i].Embedding
	}

	return result, false, nil
}

// EmbedOne is a convenience method that embeds a single text.
func (c *Client) EmbedOne(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := c.embed(ctx, []string{text}, "")
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return []float32{}, nil
	}
	return embeddings[0], nil
}
