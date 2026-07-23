package milx

import (
	"sync"
)

type CacheKey struct {
	GenerationID           string
	ExtensionID            string
	ExtensionVersion       string
	ExecutableSHA256       string
	Operation              string
	ParametersDigest       string
	AuthorityProfileDigest string
	OutputSchema           string
}

type AnalysisCache struct {
	mu     sync.RWMutex
	max    int
	values map[CacheKey][]byte
}

func NewAnalysisCache(maxBytes int) *AnalysisCache {
	if maxBytes < 0 {
		maxBytes = 0
	}
	return &AnalysisCache{max: maxBytes, values: make(map[CacheKey][]byte)}
}
func (c *AnalysisCache) Put(k CacheKey, value []byte) error {
	if len(value) > c.max {
		return NewError("GPH_MILX_OUTPUT_INVALID", "cache", false, "", "cache value exceeds configured bound")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[k] = append([]byte(nil), value...)
	return nil
}
func (c *AnalysisCache) Get(k CacheKey) ([]byte, bool) {
	c.mu.RLock()
	value, ok := c.values[k]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return append([]byte(nil), value...), true
}
