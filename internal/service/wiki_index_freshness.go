package service

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func indexedDocContentIsCurrent(record model.DocRecord, content string) bool {
	if strings.TrimSpace(record.ContentHash) == "" {
		return true
	}
	sum := sha1.Sum([]byte(content))
	return strings.EqualFold(record.ContentHash, hex.EncodeToString(sum[:]))
}
