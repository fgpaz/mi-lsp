package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

type qCursorClaims struct {
	Query      string `json:"q"`
	Workspace  string `json:"ws"`
	Generation string `json:"gen"`
	Offset     int    `json:"off"`
	Expires    int64  `json:"exp"`
}

var qCursorKey [32]byte
var qCursorKeyOnce sync.Once

func initQCursorKey() {
	qCursorKeyOnce.Do(func() {
		if _, err := rand.Read(qCursorKey[:]); err != nil {
			sum := sha256.Sum256([]byte(time.Now().UTC().String()))
			qCursorKey = sum
		}
	})
}
func qQueryDigest(query string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(query)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
func signQCursor(query, workspace, generation string, offset int) string {
	initQCursorKey()
	claims := qCursorClaims{Query: qQueryDigest(query), Workspace: workspace, Generation: generation, Offset: offset, Expires: time.Now().Add(10 * time.Minute).Unix()}
	raw, _ := json.Marshal(claims)
	body := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, qCursorKey[:])
	_, _ = mac.Write([]byte(body))
	return "q1." + body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func verifyQCursor(token, query, workspace string) (qCursorClaims, string) {
	initQCursorKey()
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "q1" {
		return qCursorClaims{}, "cursor_invalid"
	}
	mac := hmac.New(sha256.New, qCursorKey[:])
	_, _ = mac.Write([]byte(parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return qCursorClaims{}, "cursor_invalid"
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return qCursorClaims{}, "cursor_invalid"
	}
	var claims qCursorClaims
	if json.Unmarshal(raw, &claims) != nil {
		return qCursorClaims{}, "cursor_invalid"
	}
	if claims.Expires < time.Now().Unix() {
		return qCursorClaims{}, "cursor_expired"
	}
	if claims.Query != qQueryDigest(query) || claims.Workspace != workspace || claims.Offset < 0 {
		return qCursorClaims{}, "cursor_invalid"
	}
	return claims, ""
}
