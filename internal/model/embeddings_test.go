package model

import "testing"

func TestEmbeddingsBlockActive(t *testing.T) {
	explicitTrue := true
	explicitFalse := false

	tests := []struct {
		name string
		cfg  *EmbeddingsBlock
		want bool
	}{
		{
			name: "nil block disabled",
			cfg:  nil,
			want: false,
		},
		{
			name: "base url and model activate when enabled omitted",
			cfg: &EmbeddingsBlock{
				BaseURL: "http://localhost:11434/v1",
				Model:   "bge-m3",
			},
			want: true,
		},
		{
			name: "explicit false disables configured block",
			cfg: &EmbeddingsBlock{
				Enabled: &explicitFalse,
				BaseURL: "http://localhost:11434/v1",
				Model:   "bge-m3",
			},
			want: false,
		},
		{
			name: "explicit true still requires base url and model",
			cfg: &EmbeddingsBlock{
				Enabled: &explicitTrue,
				BaseURL: "http://localhost:11434/v1",
			},
			want: false,
		},
		{
			name: "whitespace config remains inactive",
			cfg: &EmbeddingsBlock{
				BaseURL: " ",
				Model:   "bge-m3",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.Active(); got != tt.want {
				t.Fatalf("Active() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRerankExtensionBlockActive(t *testing.T) {
	explicitTrue := true
	explicitFalse := false

	tests := []struct {
		name string
		cfg  *RerankExtensionBlock
		want bool
	}{
		{
			name: "nil block disabled",
			cfg:  nil,
			want: false,
		},
		{
			name: "enabled omitted stays disabled",
			cfg: &RerankExtensionBlock{
				Command: "rerank-local",
			},
			want: false,
		},
		{
			name: "explicit false disables configured command",
			cfg: &RerankExtensionBlock{
				Enabled: &explicitFalse,
				Command: "rerank-local",
			},
			want: false,
		},
		{
			name: "explicit true requires command",
			cfg: &RerankExtensionBlock{
				Enabled: &explicitTrue,
				Command: " ",
			},
			want: false,
		},
		{
			name: "explicit true with command activates",
			cfg: &RerankExtensionBlock{
				Enabled: &explicitTrue,
				Command: "rerank-local",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.Active(); got != tt.want {
				t.Fatalf("Active() = %v, want %v", got, tt.want)
			}
		})
	}
}
