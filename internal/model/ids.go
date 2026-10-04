package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// Identificadores estables id-v1. Son autodescriptivos: se resuelven contra el
// catálogo sin migrar el schema y sobreviven a un reindex (no usan el rowid).
//
//	s1:<file>#<qualified_name>[~<sig4>]@<rev8>   símbolo
//	r1:<file>:<a>-<b>@<rev8>                      rango de líneas
//	d1:<doc_id>@<rev8>                            documento
const (
	IDSchemeSymbol = "s1"
	IDSchemeRange  = "r1"
	IDSchemeDoc    = "d1"

	// IDRevLen es la cantidad de hex del hash de contenido (rev8).
	IDRevLen = 8
	// IDSigLen es la cantidad de hex de signature_hash que desambigua sobrecargas.
	IDSigLen = 4
)

// Estados de resolución de un id contra el contenido actual.
const (
	IDStateFresh   = "fresh"
	IDStateStale   = "stale"
	IDStateMissing = "missing"
)

// ID es un identificador id-v1 ya descompuesto. Rev vacío es válido: el id se
// resuelve y vuelve con su rev actual.
type ID struct {
	Scheme string
	File   string // s1, r1
	Name   string // s1: qualified_name; d1: doc_id
	Sig    string // s1: ~sig4 (solo con sobrecargas)
	Start  int    // r1
	End    int    // r1
	Rev    string
}

// RevOf devuelve rev8: los primeros 8 hex del sha256 del texto normalizado a "\n".
func RevOf(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:IDRevLen]
}

// RevOfLines calcula rev8 sobre un rango de líneas (sin terminador final).
func RevOfLines(lines []string) string {
	return RevOf(strings.Join(lines, "\n"))
}

// SigOf toma los 4 primeros hex de signature_hash.
func SigOf(signatureHash string) string {
	signatureHash = strings.ToLower(strings.TrimSpace(signatureHash))
	if len(signatureHash) > IDSigLen {
		signatureHash = signatureHash[:IDSigLen]
	}
	return signatureHash
}

// SymbolQName es el qualified_name que viaja en el id: sin el prefijo
// "<file>::" del catálogo. Si el catálogo no lo trae, se arma con parent.name.
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

// SymbolID arma el id de un símbolo. withSig agrega ~sig4 (solo con sobrecargas).
func SymbolID(rec SymbolRecord, rev string, withSig bool) ID {
	id := ID{Scheme: IDSchemeSymbol, File: rec.FilePath, Name: SymbolQName(rec), Rev: rev}
	if withSig {
		id.Sig = SigOf(rec.SignatureHash)
	}
	return id
}

// RangeID arma el id de un rango de líneas.
func RangeID(file string, start, end int, rev string) ID {
	if end < start {
		end = start
	}
	return ID{Scheme: IDSchemeRange, File: file, Start: start, End: end, Rev: rev}
}

// DocumentID arma el id de un documento.
func DocumentID(docID string, rev string) ID {
	return ID{Scheme: IDSchemeDoc, Name: docID, Rev: rev}
}

// String formatea el id en su forma canónica.
func (id ID) String() string {
	var b strings.Builder
	b.WriteString(id.Scheme)
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

// WithRev devuelve una copia con otro rev.
func (id ID) WithRev(rev string) ID {
	id.Rev = rev
	return id
}

// Base es el id sin rev: la identidad lógica (clave de re-resolución).
func (id ID) Base() string { return id.WithRev("").String() }

// ParseID descompone un id-v1. Un id sin "@rev" es válido.
func ParseID(raw string) (ID, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) < 4 || raw[2] != ':' {
		return ID{}, fmt.Errorf("id-v1: se esperaba s1:, r1: o d1: en %q", truncateForError(raw))
	}
	id := ID{Scheme: raw[:2]}
	body := raw[3:]
	if id.Scheme != IDSchemeSymbol && id.Scheme != IDSchemeRange && id.Scheme != IDSchemeDoc {
		return ID{}, fmt.Errorf("id-v1: esquema %q desconocido (s1, r1, d1)", id.Scheme)
	}
	if n := len(body); n > IDRevLen+1 && body[n-IDRevLen-1] == '@' && isHex(body[n-IDRevLen:]) {
		id.Rev = body[n-IDRevLen:]
		body = body[:n-IDRevLen-1]
	}
	if body == "" {
		return ID{}, fmt.Errorf("id-v1: %s sin cuerpo", id.Scheme)
	}
	switch id.Scheme {
	case IDSchemeSymbol:
		hash := strings.Index(body, "#")
		if hash <= 0 || hash == len(body)-1 {
			return ID{}, fmt.Errorf("id-v1: s1 requiere <file>#<qualified_name>")
		}
		id.File = body[:hash]
		id.Name = body[hash+1:]
		if n := len(id.Name); n > IDSigLen+1 && id.Name[n-IDSigLen-1] == '~' && isHex(id.Name[n-IDSigLen:]) {
			id.Sig = id.Name[n-IDSigLen:]
			id.Name = id.Name[:n-IDSigLen-1]
		}
	case IDSchemeRange:
		dash := strings.LastIndex(body, "-")
		colon := strings.LastIndex(body, ":")
		if colon <= 0 || dash < colon+2 || dash == len(body)-1 {
			return ID{}, fmt.Errorf("id-v1: r1 requiere <file>:<a>-<b>")
		}
		start, errA := strconv.Atoi(body[colon+1 : dash])
		end, errB := strconv.Atoi(body[dash+1:])
		if errA != nil || errB != nil || start < 1 || end < start {
			return ID{}, fmt.Errorf("id-v1: rango r1 inválido")
		}
		id.File, id.Start, id.End = body[:colon], start, end
	case IDSchemeDoc:
		id.Name = body
	}
	return id, nil
}

// IsID indica si el texto tiene forma de id-v1 (esquema válido y cuerpo parseable).
func IsID(raw string) bool {
	_, err := ParseID(raw)
	return err == nil
}

// ResolveSymbolID busca el símbolo de un id s1 entre los candidatos del archivo.
// revOf calcula el rev actual de un candidato. Devuelve el símbolo, su rev actual
// y el estado: fresh (rev coincide o el id no traía rev), stale (se re-resolvió
// por file#qualified_name con otro rev) o missing (ya no existe; no es error).
func ResolveSymbolID(id ID, candidates []SymbolRecord, revOf func(SymbolRecord) string) (SymbolRecord, string, string) {
	var matches []SymbolRecord
	for _, cand := range candidates {
		if cand.FilePath == id.File && SymbolQName(cand) == id.Name {
			matches = append(matches, cand)
		}
	}
	if id.Sig != "" && len(matches) > 1 {
		filtered := matches[:0:0]
		for _, cand := range matches {
			if SigOf(cand.SignatureHash) == id.Sig {
				filtered = append(filtered, cand)
			}
		}
		matches = filtered
	}
	if len(matches) == 0 {
		return SymbolRecord{}, "", IDStateMissing
	}
	chosen := matches[0]
	if len(matches) > 1 {
		for _, cand := range matches {
			if id.Rev != "" && revOf(cand) == id.Rev {
				chosen = cand
				break
			}
		}
	}
	rev := revOf(chosen)
	if id.Rev == "" || id.Rev == rev {
		return chosen, rev, IDStateFresh
	}
	return chosen, rev, IDStateStale
}

func isHex(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func truncateForError(value string) string {
	if len(value) > 40 {
		return value[:40] + "..."
	}
	return value
}
