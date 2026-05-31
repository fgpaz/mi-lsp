package embed

import (
	"math"
	"testing"
)

func TestEncodeDecodeVectorRoundTrip(t *testing.T) {
	original := []float32{1.5, -2.3, 0.0, 3.14159}

	encoded := EncodeVector(original)
	decoded := DecodeVector(encoded)

	if len(decoded) != len(original) {
		t.Fatalf("length mismatch: got %d, expected %d", len(decoded), len(original))
	}

	for i := range original {
		// Allow for small floating-point precision loss
		if math.Abs(float64(decoded[i]-original[i])) > 1e-6 {
			t.Errorf("value mismatch at index %d: got %f, expected %f", i, decoded[i], original[i])
		}
	}
}

func TestCosineIdenticalNormalized(t *testing.T) {
	// A normalized vector (norm = 1)
	v := []float32{0.6, 0.8}

	result := Cosine(v, v)
	expected := 1.0

	if math.Abs(result-expected) > 1e-6 {
		t.Errorf("cosine of identical vectors: got %f, expected %f", result, expected)
	}
}

func TestCosineOrthogonal(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}

	result := Cosine(a, b)
	expected := 0.0

	if math.Abs(result-expected) > 1e-6 {
		t.Errorf("cosine of orthogonal vectors: got %f, expected %f", result, expected)
	}
}

func TestCosineMismatchedLength(t *testing.T) {
	a := []float32{1.0, 2.0}
	b := []float32{3.0, 4.0, 5.0}

	result := Cosine(a, b)
	expected := 0.0

	if result != expected {
		t.Errorf("cosine of mismatched lengths: got %f, expected %f", result, expected)
	}
}

func TestCosineEmptyVectors(t *testing.T) {
	emptyA := []float32{}
	emptyB := []float32{}
	nonEmpty := []float32{1.0, 2.0}

	resultEmpty := Cosine(emptyA, emptyB)
	resultEmptyNonEmpty := Cosine(emptyA, nonEmpty)
	resultNonEmptyEmpty := Cosine(nonEmpty, emptyB)

	if resultEmpty != 0 {
		t.Errorf("cosine of two empty vectors: got %f, expected 0", resultEmpty)
	}
	if resultEmptyNonEmpty != 0 {
		t.Errorf("cosine of empty and non-empty: got %f, expected 0", resultEmptyNonEmpty)
	}
	if resultNonEmptyEmpty != 0 {
		t.Errorf("cosine of non-empty and empty: got %f, expected 0", resultNonEmptyEmpty)
	}
}

func TestCosineZeroNorm(t *testing.T) {
	zero := []float32{0.0, 0.0, 0.0}
	nonZero := []float32{1.0, 2.0, 3.0}

	result := Cosine(zero, nonZero)
	expected := 0.0

	if result != expected {
		t.Errorf("cosine with zero norm: got %f, expected %f", result, expected)
	}
}

func TestDecodeVectorWithTrailingBytes(t *testing.T) {
	// Create a vector with 2 floats (8 bytes) + 1 trailing byte
	v := []float32{1.5, 2.5}
	encoded := EncodeVector(v)
	// Append a trailing byte
	encoded = append(encoded, 0xFF)

	decoded := DecodeVector(encoded)

	if len(decoded) != 2 {
		t.Fatalf("length should be 2, got %d", len(decoded))
	}

	for i := range v {
		if math.Abs(float64(decoded[i]-v[i])) > 1e-6 {
			t.Errorf("value mismatch at index %d: got %f, expected %f", i, decoded[i], v[i])
		}
	}
}
