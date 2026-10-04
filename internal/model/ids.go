package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

// q-v1 identities carry workspace identity; content revision is an independent
// field. Legacy s1/r1/d1 IDs remain parseable solely for transition resolution.
const (
	IDSchemeSymbol = "symbol"
	IDSchemeRange  = "range"
	IDSchemeDoc    = "doc"
	IDRevLen       = 64
	IDSigLen       = 4
)

const (
	IDStateFresh   = "fresh"
	IDStateStale   = "stale"
	IDStateMissing = "missing"
)

type ID struct {
	Scheme      string
	WorkspaceID string
	File        string
	Name        string
	Sig         string
	Start       int
	End         int
	Rev         string
	Legacy      bool
}

// WorkspaceID is stable for aliases of the same absolute workspace root.
func WorkspaceID(root string) string {
	root = filepath.Clean(strings.ReplaceAll(root, "\\", "/"))
	if absolute, err := filepath.Abs(root); err == nil {
		root = filepath.Clean(strings.ReplaceAll(absolute, "\\", "/"))
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = filepath.Clean(strings.ReplaceAll(resolved, "\\", "/"))
	}
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:])
}

// RevOf returns the full SHA-256 of content normalized to LF and without a
// trailing line terminator.
func RevOf(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSuffix(text, "\n")
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func RevOfLines(lines []string) string { return RevOf(strings.Join(lines, "\n")) }

func SigOf(signatureHash string) string {
	signatureHash = strings.ToLower(strings.TrimSpace(signatureHash))
	if len(signatureHash) > IDSigLen {
		signatureHash = signatureHash[:IDSigLen]
	}
	return signatureHash
}

func SymbolQName(rec SymbolRecord) string {
	name := strings.TrimSpace(rec.QualifiedName)
	if name == "" {
		if rec.Parent != "" {
			return rec.Parent + "." + rec.Name
		}
		return rec.Name
	}
	if rec.FilePath != "" {
		name = strings.TrimPrefix(name, rec.FilePath+"::")
	}
	return name
}

func StableSymbolID(workspaceID string, rec SymbolRecord, rev string, withSig bool) ID {
	id := ID{Scheme: IDSchemeSymbol, WorkspaceID: workspaceID, File: rec.FilePath, Name: SymbolQName(rec), Rev: rev}
	if withSig {
		candidate := strings.ToLower(strings.TrimSpace(rec.SignatureHash))
		if len(candidate) == 64 && isHex(candidate) {
			id.Sig = candidate
		}
	}
	return id
}

func StableRangeID(workspaceID, file string, start, end int, rev string) ID {
	if end < start {
		end = start
	}
	return ID{Scheme: IDSchemeRange, WorkspaceID: workspaceID, File: file, Start: start, End: end, Rev: rev}
}

func StableDocumentID(workspaceID, file, docID, rev string) ID {
	if file == "" {
		file = "_wiki/" + docID
	}
	return ID{Scheme: IDSchemeDoc, WorkspaceID: workspaceID, File: file, Name: docID, Rev: rev}
}

// Deprecated id-v1 constructors are retained for source compatibility; q-v1
// execution must call the Stable* constructors with a workspace identity.
func SymbolID(rec SymbolRecord, rev string, withSig bool) ID {
	id := StableSymbolID("", rec, rev, false)
	if withSig {
		id.Sig = SigOf(rec.SignatureHash)
	}
	id.Legacy = true
	return id
}
func RangeID(file string, start, end int, rev string) ID {
	id := StableRangeID("", file, start, end, rev)
	id.Legacy = true
	return id
}
func DocumentID(docID string, rev string) ID {
	id := StableDocumentID("", "", docID, rev)
	id.Legacy = true
	return id
}

func (id ID) String() string {
	if id.Legacy || id.WorkspaceID == "" {
		return id.legacyString()
	}
	var symbol string
	switch id.Scheme {
	case IDSchemeSymbol:
		symbol = id.Name
		if id.Sig != "" {
			symbol += "~" + strings.ToLower(id.Sig)
		}
	case IDSchemeRange:
		symbol = strconv.Itoa(id.Start) + "-" + strconv.Itoa(id.End)
	case IDSchemeDoc:
		symbol = id.Name
	default:
		return ""
	}
	return "ws:" + id.WorkspaceID + "/" + id.Scheme + "/" + encodeIDPath(id.File) + "#" + encodeIDPart(symbol)
}

func (id ID) legacyString() string {
	var b strings.Builder
	prefix := map[string]string{IDSchemeSymbol: "s1", IDSchemeRange: "r1", IDSchemeDoc: "d1"}[id.Scheme]
	if prefix == "" {
		prefix = id.Scheme
	}
	b.WriteString(prefix)
	b.WriteByte(':')
	switch id.Scheme {
	case IDSchemeSymbol:
		b.WriteString(id.File)
		b.WriteByte('#')
		b.WriteString(id.Name)
		if id.Sig != "" {
			b.WriteByte('~')
			b.WriteString(id.Sig)
		}
	case IDSchemeRange:
		b.WriteString(id.File)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(id.Start))
		b.WriteByte('-')
		b.WriteString(strconv.Itoa(id.End))
	case IDSchemeDoc:
		b.WriteString(id.Name)
	}
	if id.Rev != "" {
		b.WriteByte('@')
		b.WriteString(id.Rev)
	}
	return b.String()
}

func (id ID) WithRev(rev string) ID { id.Rev = strings.TrimPrefix(rev, "rev:"); return id }
func (id ID) Base() string          { return id.WithRev("").String() }

func ParseID(raw string) (ID, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "ws:") {
		return parseStableID(raw)
	}
	return parseLegacyID(raw)
}

func parseStableID(raw string) (ID, error) {
	body := strings.TrimPrefix(raw, "ws:")
	parts := strings.SplitN(body, "/", 3)
	if len(parts) != 3 || len(parts[0]) != 64 || !isHex(parts[0]) {
		return ID{}, fmt.Errorf("q-v1 id: workspace_id debe ser SHA-256 hexadecimal")
	}
	hash := strings.IndexByte(parts[2], '#')
	if hash < 0 {
		return ID{}, fmt.Errorf("q-v1 id: falta separador #")
	}
	kind := parts[1]
	if kind != IDSchemeSymbol && kind != IDSchemeRange && kind != IDSchemeDoc {
		return ID{}, fmt.Errorf("q-v1 id: kind desconocido %q", kind)
	}
	file, err := decodeIDPath(parts[2][:hash])
	if err != nil {
		return ID{}, err
	}
	symbol, err := decodeIDPart(parts[2][hash+1:])
	if err != nil || symbol == "" {
		return ID{}, fmt.Errorf("q-v1 id: symbol inválido")
	}
	id := ID{Scheme: kind, WorkspaceID: parts[0], File: file}
	switch kind {
	case IDSchemeSymbol:
		id.Name = symbol
		if at := strings.LastIndex(symbol, "~"); at >= 0 {
			sig := symbol[at+1:]
			if len(sig) != 64 || !isHex(sig) {
				return ID{}, fmt.Errorf("q-v1 id: signature_hash inválido")
			}
			id.Name, id.Sig = symbol[:at], strings.ToLower(sig)
		}
	case IDSchemeRange:
		dash := strings.IndexByte(symbol, '-')
		if dash < 1 {
			return ID{}, fmt.Errorf("q-v1 id: range requiere inicio-fin")
		}
		var e1, e2 error
		id.Start, e1 = strconv.Atoi(symbol[:dash])
		id.End, e2 = strconv.Atoi(symbol[dash+1:])
		if e1 != nil || e2 != nil || id.Start < 1 || id.End < id.Start {
			return ID{}, fmt.Errorf("q-v1 id: range inválido")
		}
	case IDSchemeDoc:
		id.Name = symbol
	}
	return id, nil
}

func parseLegacyID(raw string) (ID, error) {
	if len(raw) < 4 || raw[2] != ':' {
		return ID{}, fmt.Errorf("id: se esperaba ws:, s1:, r1: o d1:")
	}
	prefix, body := raw[:2], raw[3:]
	id := ID{Legacy: true}
	if at := strings.LastIndexByte(body, '@'); at >= 0 {
		rev := body[at+1:]
		if (len(rev) == 8 || len(rev) == IDRevLen) && isHex(rev) {
			id.Rev = rev
			body = body[:at]
		}
	}
	switch prefix {
	case "s1":
		id.Scheme = IDSchemeSymbol
	case "r1":
		id.Scheme = IDSchemeRange
	case "d1":
		id.Scheme = IDSchemeDoc
	default:
		return ID{}, fmt.Errorf("id legacy: esquema desconocido")
	}
	if id.Scheme == IDSchemeDoc {
		id.Name = body
	} else if id.Scheme == IDSchemeSymbol {
		sep := strings.IndexByte(body, '#')
		if sep < 1 || sep == len(body)-1 {
			return ID{}, fmt.Errorf("id legacy: s1 requiere archivo#símbolo")
		}
		id.File, id.Name = body[:sep], body[sep+1:]
		if tilde := strings.LastIndexByte(id.Name, '~'); tilde >= 0 {
			sig := id.Name[tilde+1:]
			if (len(sig) == 4 || len(sig) == 64) && isHex(sig) {
				id.Name, id.Sig = id.Name[:tilde], sig
			}
		}
	} else {
		colon, dash := strings.LastIndex(body, ':'), strings.LastIndex(body, '-')
		if colon < 1 || dash <= colon {
			return ID{}, fmt.Errorf("id legacy: rango inválido")
		}
		var errA, errB error
		id.File = body[:colon]
		id.Start, errA = strconv.Atoi(body[colon+1 : dash])
		id.End, errB = strconv.Atoi(body[dash+1:])
		if errA != nil || errB != nil || id.Start < 1 || id.End < id.Start {
			return ID{}, fmt.Errorf("id legacy: rango inválido")
		}
	}
	return id, nil
}

func IsID(raw string) bool { _, err := ParseID(raw); return err == nil }

func ResolveSymbolID(id ID, candidates []SymbolRecord, revOf func(SymbolRecord) string) (SymbolRecord, string, string) {
	var matches []SymbolRecord
	for _, candidate := range candidates {
		if candidate.FilePath == id.File && SymbolQName(candidate) == id.Name {
			matches = append(matches, candidate)
		}
	}
	if id.Sig != "" && len(matches) > 1 {
		filtered := matches[:0:0]
		for _, candidate := range matches {
			if strings.HasPrefix(strings.ToLower(candidate.SignatureHash), strings.ToLower(id.Sig)) {
				filtered = append(filtered, candidate)
			}
		}
		matches = filtered
	}
	if len(matches) == 0 {
		return SymbolRecord{}, "", IDStateMissing
	}
	chosen := matches[0]
	if len(matches) > 1 {
		for _, candidate := range matches {
			if revOf(candidate) == id.Rev && id.Rev != "" {
				chosen = candidate
				break
			}
		}
	}
	rev := revOf(chosen)
	if id.Rev == "" || strings.TrimPrefix(id.Rev, "rev:") == rev {
		return chosen, rev, IDStateFresh
	}
	return chosen, rev, IDStateStale
}

func encodeIDPart(value string) string {
	const hexDigits = "0123456789ABCDEF"
	var out strings.Builder
	for _, b := range []byte(value) {
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || strings.ContainsRune("-._~", rune(b)) {
			out.WriteByte(b)
			continue
		}
		out.WriteByte('%')
		out.WriteByte(hexDigits[b>>4])
		out.WriteByte(hexDigits[b&15])
	}
	return out.String()
}
func encodeIDPath(value string) string {
	parts := strings.Split(strings.ReplaceAll(value, "\\", "/"), "/")
	for i := range parts {
		parts[i] = encodeIDPart(parts[i])
	}
	return strings.Join(parts, "/")
}
func decodeIDPath(value string) (string, error) {
	parts := strings.Split(value, "/")
	for i := range parts {
		decoded, err := decodeIDPart(parts[i])
		if err != nil {
			return "", err
		}
		parts[i] = decoded
	}
	return strings.Join(parts, "/"), nil
}
func decodeIDPart(value string) (string, error) { return url.PathUnescape(value) }

func isHex(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
