package embed

import (
	"encoding/binary"
	"math"
)

// EncodeVector encodes a slice of float32 values as little-endian bytes.
func EncodeVector(v []float32) []byte {
	if len(v) == 0 {
		return []byte{}
	}
	result := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(result[i*4:], math.Float32bits(f))
	}
	return result
}

// DecodeVector decodes little-endian bytes back to a slice of float32 values.
// If len(b) is not a multiple of 4, trailing bytes are ignored.
func DecodeVector(b []byte) []float32 {
	if len(b) == 0 {
		return []float32{}
	}
	numFloats := len(b) / 4
	result := make([]float32, numFloats)
	for i := 0; i < numFloats; i++ {
		bits := binary.LittleEndian.Uint32(b[i*4 : i*4+4])
		result[i] = math.Float32frombits(bits)
	}
	return result
}

// Cosine computes the cosine similarity between two vectors.
// Returns 0 if either vector is empty, has zero norm, or vectors have different lengths.
// Otherwise returns dot(a, b) / (norm(a) * norm(b)).
func Cosine(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		fa := float64(a[i])
		fb := float64(b[i])
		dot += fa * fb
		normA += fa * fa
		normB += fb * fb
	}

	normA = math.Sqrt(normA)
	normB = math.Sqrt(normB)

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (normA * normB)
}
