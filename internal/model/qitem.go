package model

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// Contrato q-v1: ítems, etapas y presupuesto de las primitivas y del lenguaje q.

// QDefaultFields es la proyección por defecto de un ítem.
var QDefaultFields = []string{"id", "kind", "name", "file", "line", "origin"}

// QEdge describe de dónde viene un ítem producido por `edges`.
type QEdge struct {
	Dir   string `json:"dir"`
	From  string `json:"from"`
	Depth int    `json:"depth"`
}

// QItem es el ítem interno de las primitivas. Se proyecta a QRow antes de salir.
type QItem struct {
	ID            string
	Revision      string
	TruncatedItem bool
	Kind          string
	Name          string
	File          string
	Line          int
	EndLine       int
	Origin        string
	Lang          string
	Parent        string
	Signature     string
	Title         string
	Layer         string
	Score         int
	Text          string
	Read          bool
	// Estado de resolución y de sesión.
	Stale   bool
	Missing bool
	Seen    bool
	// Edge e In solo existen en ítems producidos por `edges`/`text`.
	Edge *QEdge
	In   string
}

// Rev devuelve la revisión SHA-256 separada del identificador.
func (it QItem) Rev() string {
	if it.Revision != "" {
		return strings.TrimPrefix(it.Revision, "rev:")
	}
	if parsed, err := ParseID(it.ID); err == nil {
		return parsed.Rev
	}
	return ""
}

// QFieldNames son los campos proyectables.
var QFieldNames = []string{"id", "rev", "kind", "name", "file", "path", "line", "end_line", "origin", "lang", "parent", "signature", "title", "layer", "score", "text", "edge", "in", "stale", "missing", "seen", "truncated_item"}

// QFieldValid indica si name es un campo conocido de ítem.
func QFieldValid(name string) bool {
	for _, field := range QFieldNames {
		if field == name {
			return true
		}
	}
	return false
}

// Field devuelve el valor de un campo y si está presente (no vacío).
func (it QItem) Field(name string) (any, bool) {
	switch name {
	case "id":
		return it.ID, it.ID != ""
	case "kind":
		return it.Kind, it.Kind != ""
	case "name":
		return it.Name, it.Name != ""
	case "file", "path":
		return it.File, it.File != ""
	case "line":
		return it.Line, it.Line > 0
	case "end_line":
		return it.EndLine, it.EndLine > 0
	case "origin":
		return it.Origin, it.Origin != ""
	case "lang":
		return it.Lang, it.Lang != ""
	case "parent":
		return it.Parent, it.Parent != ""
	case "signature":
		return it.Signature, it.Signature != ""
	case "title":
		return it.Title, it.Title != ""
	case "layer":
		return it.Layer, it.Layer != ""
	case "score":
		return it.Score, it.Score != 0
	case "text":
		return it.Text, it.Text != ""
	case "edge":
		return it.Edge, it.Edge != nil
	case "in":
		return it.In, it.In != ""
	case "rev":
		rev := it.Rev()
		if rev == "" {
			return nil, false
		}
		return "rev:" + rev, true
	case "truncated_item":
		return it.TruncatedItem, it.TruncatedItem
	case "stale":
		return it.Stale, it.Stale
	case "missing":
		return it.Missing, it.Missing
	case "seen":
		return it.Seen, it.Seen
	}
	return nil, false
}

// QRow es un ítem ya proyectado: claves en orden estable para el JSON.
type QRow struct {
	keys []string
	vals map[string]any
}

// Project arma la fila de un ítem. fields vacío usa QDefaultFields. Siempre se
// agregan los marcadores de estado (stale, missing, seen); edge e in se proyectan
// por defecto y text solo tras `read`.
func (it QItem) Project(fields []string) QRow {
	row := QRow{vals: map[string]any{}}
	add := func(name string) {
		if _, exists := row.vals[name]; exists {
			return
		}
		if value, ok := it.Field(name); ok {
			row.keys = append(row.keys, name)
			row.vals[name] = value
		}
	}
	if len(fields) == 0 {
		for _, name := range QDefaultFields {
			add(name)
		}
		add("edge")
		add("in")
		if it.Read {
			add("text")
		}
	} else {
		for _, name := range fields {
			add(name)
		}
	}
	if it.Stale || it.Seen {
		add("rev")
	}
	add("stale")
	add("missing")
	add("seen")
	add("truncated_item")
	return row
}

// Get lee un valor de la fila.
func (r QRow) Get(name string) (any, bool) {
	value, ok := r.vals[name]
	return value, ok
}

// Keys devuelve las claves en orden.
func (r QRow) Keys() []string { return append([]string(nil), r.keys...) }

// MarshalJSON respeta el orden de las claves.
func (r QRow) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range r.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		name, _ := json.Marshal(key)
		buf.Write(name)
		buf.WriteByte(':')
		value, err := marshalNoEscape(r.vals[key])
		if err != nil {
			return nil, err
		}
		buf.Write(value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func marshalNoEscape(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// JSONSize es el tamaño en bytes del ítem serializado como JSON.
func (r QRow) JSONSize() int {
	encoded, err := r.MarshalJSON()
	if err != nil {
		return 0
	}
	return len(encoded)
}

// Line es la forma compacta de una línea:
// "s1:…@rev  func  App.Execute  internal/service/app.go:56". El texto (tras `read`)
// va debajo, con sangría.
func (r QRow) Line() string {
	var parts []string
	if id, ok := r.vals["id"].(string); ok {
		parts = append(parts, id)
	}
	if kind, ok := r.vals["kind"].(string); ok {
		parts = append(parts, kind)
	}
	if name, ok := r.vals["name"].(string); ok {
		parts = append(parts, name)
	}
	if file, ok := r.vals["file"].(string); ok {
		location := file
		if line, ok := r.vals["line"].(int); ok {
			location += ":" + strconv.Itoa(line)
		}
		parts = append(parts, location)
	}
	skip := map[string]bool{"id": true, "kind": true, "name": true, "file": true, "line": true, "text": true}
	var extra []string
	for _, key := range r.keys {
		if skip[key] {
			continue
		}
		value := r.vals[key]
		switch typed := value.(type) {
		case bool:
			extra = append(extra, key)
		case string:
			extra = append(extra, key+"="+typed)
		case *QEdge:
			if typed != nil {
				extra = append(extra, "edge="+typed.Dir+"<"+typed.From+" d"+strconv.Itoa(typed.Depth))
			}
		default:
			encoded, _ := json.Marshal(value)
			extra = append(extra, key+"="+string(encoded))
		}
	}
	line := strings.Join(parts, "  ")
	if len(extra) > 0 {
		line += "  [" + strings.Join(extra, " ") + "]"
	}
	if text, ok := r.vals["text"].(string); ok && text != "" {
		line += "\n" + indentLines(text, "    ")
	}
	return line
}

func indentLines(text, prefix string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

// QItems es la lista de filas que viaja en Envelope.Items de la operación q.
// Es un tipo propio para que los formateadores de nav.* no la reinterpreten.
type QItems []QRow

// Lines renderiza una línea por ítem.
func (q QItems) Lines() []string {
	lines := make([]string, len(q))
	for i, row := range q {
		lines[i] = row.Line()
	}
	return lines
}

// QStage es el resumen de una etapa de la pipeline.
type QStage struct {
	Verb      string `json:"verb"`
	In        int    `json:"in"`
	Out       int    `json:"out"`
	Ms        int64  `json:"ms"`
	Truncated bool   `json:"truncated,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// QBudget informa el presupuesto pedido y el realmente usado (tokens estimados).
type QBudget struct {
	Requested int `json:"requested"`
	Used      int `json:"used"`
}

// Reason adicionales de q-v1 (aditivos al catálogo de primitives-v2).
const (
	ReasonGraphUnavailable   = "graph_unavailable"
	ReasonSessionUnavailable = "session_unavailable"
	ReasonQTimeout           = "q_timeout"
	ReasonUnsupportedOp      = "unsupported_operation"
)

// qReasonRank ordena los reason de etapas por severidad: gana el mayor.
var qReasonRank = map[string]int{
	ReasonIndexSchemaBroken:     90,
	ReasonIndexNotReady:         80,
	ReasonLSPUnavailable:        70,
	ReasonLSPError:              60,
	ReasonGraphUnavailable:      55,
	ReasonSessionUnavailable:    52,
	ReasonQTimeout:              50,
	ReasonLanguageUnsupported:   40,
	ReasonSemanticEmptyTextHits: 30,
	ReasonNoMatches:             1,
}

// MoreSevereReason devuelve el más severo de dos reason (vacío pierde).
func MoreSevereReason(a, b string) string {
	if qReasonRank[b] > qReasonRank[a] || (a == "" && b != "") {
		return b
	}
	return a
}

// SortQItems ordena por una clave de ítem de forma estable (para `sort`).
func SortQItems(items []QItem, field string, desc bool) {
	less := func(a, b QItem) bool {
		va, _ := a.Field(field)
		vb, _ := b.Field(field)
		switch x := va.(type) {
		case int:
			y, _ := vb.(int)
			return x < y
		case string:
			y, _ := vb.(string)
			return x < y
		}
		return false
	}
	sort.SliceStable(items, func(i, j int) bool {
		if desc {
			return less(items[j], items[i])
		}
		return less(items[i], items[j])
	})
}
