package query

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const ContractVersion = "q-v1"
const MaxStages = 8

type Stage struct {
	Verb    string
	Args    []string
	Options map[string]string
	Flags   map[string]bool
}

type Pipeline struct {
	Stages    []Stage
	Budget    int
	MaxBytes  int
	Workspace string
	Page      string
	Fresh     bool
	Count     bool
	Fields    []string
}

func Parse(input string) (Pipeline, error) {
	parts, err := splitPipeline(input)
	if err != nil {
		return Pipeline{}, err
	}
	if len(parts) == 0 || len(parts) > MaxStages {
		return Pipeline{}, fmt.Errorf("q: se admiten de 1 a %d etapas", MaxStages)
	}
	p := Pipeline{Budget: 2000, MaxBytes: 262144}
	for index, raw := range parts {
		tokens, err := tokenize(raw)
		if err != nil {
			return Pipeline{}, fmt.Errorf("q: %d: %w", index+1, err)
		}
		if len(tokens) == 0 {
			return Pipeline{}, fmt.Errorf("q: etapa %d vacía", index+1)
		}
		stage := Stage{Verb: strings.ToLower(tokens[0]), Options: map[string]string{}, Flags: map[string]bool{}}
		for _, token := range tokens[1:] {
			if eq := strings.IndexByte(token, '='); eq > 0 {
				key, value := strings.ToLower(token[:eq]), token[eq+1:]
				switch key {
				case "budget":
					p.Budget, err = strconv.Atoi(value)
				case "max_bytes":
					p.MaxBytes, err = strconv.Atoi(value)
				case "ws", "workspace":
					p.Workspace = value
				case "page":
					p.Page = value
				case "fields":
					p.Fields = splitCSV(value)
				case "fresh":
					p.Fresh = value == "true" || value == "1"
				default:
					stage.Options[key] = value
				}
				if err != nil {
					return Pipeline{}, fmt.Errorf("q: opción %s inválida", key)
				}
			} else if token == "exact" || token == "regex" || token == "full" || token == "fresh" {
				stage.Flags[token] = true
				if token == "fresh" {
					p.Fresh = true
				}
			} else {
				stage.Args = append(stage.Args, token)
			}
		}
		if err := validateStage(stage); err != nil {
			return Pipeline{}, fmt.Errorf("q: etapa %d: %w", index+1, err)
		}
		p.Stages = append(p.Stages, stage)
	}
	if p.Budget < 1 || p.Budget > 12000 {
		return Pipeline{}, fmt.Errorf("q: budget debe estar entre 1 y 12000")
	}
	if p.MaxBytes < 1024 {
		return Pipeline{}, fmt.Errorf("q: max_bytes mínimo 1024")
	}
	if p.Fields != nil {
		for _, field := range p.Fields {
			if !fieldAllowed(field) {
				return Pipeline{}, fmt.Errorf("q: campo desconocido %q", field)
			}
		}
	}
	return expandRecipes(p)
}

func whereFieldAllowed(field string) bool {
	switch field {
	case "path", "name", "kind", "origin", "lang", "layer":
		return true
	}
	return false
}

func fieldAllowed(field string) bool {
	switch field {
	case "id", "rev", "kind", "name", "file", "path", "line", "end_line", "origin", "lang", "parent", "signature", "title", "layer", "score", "text", "edge", "in", "stale", "missing", "seen":
		return true
	}
	return false
}
func splitCSV(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func splitPipeline(s string) ([]string, error) {
	var out []string
	start := 0
	quote := rune(0)
	escaped := false
	for i, r := range s {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && quote != 0 {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == '|' {
			out = append(out, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("comillas sin cerrar")
	}
	out = append(out, strings.TrimSpace(s[start:]))
	return out, nil
}

func tokenize(s string) ([]string, error) {
	var tokens []string
	var b strings.Builder
	quote := rune(0)
	escaped := false
	active := false
	flush := func() {
		if active {
			tokens = append(tokens, b.String())
			b.Reset()
			active = false
		}
	}
	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
			active = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else if r == '\\' && quote == '"' {
				escaped = true
			} else {
				b.WriteRune(r)
			}
			active = true
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			active = true
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		b.WriteRune(r)
		active = true
	}
	if quote != 0 {
		return nil, fmt.Errorf("comillas sin cerrar")
	}
	flush()
	return tokens, nil
}

func validateStage(s Stage) error {
	if strings.HasPrefix(s.Verb, "@") {
		return nil
	}
	valid := map[string]bool{"sym": true, "text": true, "docs": true, "id": true, "diff": true, "changed": true, "edges": true, "where": true, "limit": true, "uniq": true, "sort": true, "read": true, "fields": true, "count": true, "describe": true}
	if !valid[s.Verb] {
		return fmt.Errorf("verbo desconocido %q", s.Verb)
	}
	need := func(n int) bool { return len(s.Args) >= n }
	switch s.Verb {
	case "sym", "text", "where", "sort", "fields":
		if !need(1) {
			return fmt.Errorf("%s requiere argumento", s.Verb)
		}
		if (s.Verb == "sym" || s.Verb == "text" || s.Verb == "fields") && len(s.Args) != 1 {
			return fmt.Errorf("%s recibe un argumento", s.Verb)
		}
		if s.Verb == "sort" && (len(s.Args) > 2 || len(s.Args) == 2 && s.Args[1] != "asc" && s.Args[1] != "desc") {
			return fmt.Errorf("sort acepta campo y asc|desc")
		}
		if s.Verb == "sort" && !fieldAllowed(s.Args[0]) {
			return fmt.Errorf("campo de sort no válido")
		}
		if s.Verb == "fields" {
			for _, field := range splitCSV(s.Args[0]) {
				if !fieldAllowed(field) {
					return fmt.Errorf("campo de fields no válido: %s", field)
				}
			}
		}
		if s.Verb == "where" {
			expr := strings.Join(s.Args, " ")
			found := false
			for _, op := range []string{"!~", "!=", "=", "~"} {
				if at := strings.Index(expr, op); at > 0 {
					field := strings.TrimSpace(expr[:at])
					if !whereFieldAllowed(field) {
						return fmt.Errorf("campo de where no válido: %s", field)
					}
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("where requiere campo, operador y valor")
			}
		}
	case "id":
		if !need(1) {
			return fmt.Errorf("id requiere al menos un identificador")
		}
	case "edges":
		if !need(1) {
			return fmt.Errorf("edges requiere refs|callers|callees|impl")
		}
		switch s.Args[0] {
		case "refs", "callers", "callees", "impl":
		default:
			return fmt.Errorf("edges requiere refs|callers|callees|impl")
		}
		if d := s.Options["depth"]; d != "" {
			n, e := strconv.Atoi(d)
			if e != nil || n < 1 || n > 3 {
				return fmt.Errorf("depth debe estar entre 1 y 3")
			}
		}
	case "limit":
		if !need(1) || len(s.Args) != 1 {
			return fmt.Errorf("limit requiere un entero")
		}
		n, e := strconv.Atoi(s.Args[0])
		if e != nil || n < 0 {
			return fmt.Errorf("limit requiere entero no negativo")
		}
	case "docs":
		if len(s.Args) > 1 {
			return fmt.Errorf("docs recibe una query")
		}
	case "changed":
		if s.Options["since"] == "" && len(s.Args) == 0 {
			return fmt.Errorf("changed requiere since=<mark>")
		}
		since := s.Options["since"]
		if since != "" {
			if _, err := strconv.ParseInt(since, 10, 64); err != nil {
				return fmt.Errorf("since inválido")
			}
		}
	case "read":
		for _, arg := range s.Args {
			if arg != "full" && !strings.HasPrefix(arg, "ctx=") && !strings.HasPrefix(arg, "±") {
				if _, _, _, ok := parseRange(arg); !ok {
					return fmt.Errorf("read espera ±N, ctx=N, full o file:a-b")
				}
			}
		}
		if ctx := s.Options["ctx"]; ctx != "" {
			if n, e := strconv.Atoi(ctx); e != nil || n < 0 {
				return fmt.Errorf("ctx debe ser entero no negativo")
			}
		}
	case "uniq", "count":
		if len(s.Args) > 0 {
			return fmt.Errorf("%s no recibe argumentos", s.Verb)
		}
	case "diff":
		if len(s.Args) > 0 {
			return fmt.Errorf("diff recibe ref=...")
		}
	case "describe":
		if len(s.Args) > 1 {
			return fmt.Errorf("describe recibe como máximo un selector")
		}
	}
	return nil
}
func parseRange(value string) (string, int, int, bool) {
	colon := strings.LastIndex(value, ":")
	dash := strings.LastIndex(value, "-")
	if colon < 1 || dash <= colon {
		return "", 0, 0, false
	}
	a, e1 := strconv.Atoi(value[colon+1 : dash])
	b, e2 := strconv.Atoi(value[dash+1:])
	return value[:colon], a, b, e1 == nil && e2 == nil && a > 0 && b >= a
}

func expandRecipes(p Pipeline) (Pipeline, error) {
	var out []Stage
	for _, stage := range p.Stages {
		if !strings.HasPrefix(stage.Verb, "@") {
			out = append(out, stage)
			continue
		}
		expanded, err := ExpandRecipe(stage.Verb, stage.Args)
		if err != nil {
			return Pipeline{}, err
		}
		out = append(out, expanded...)
	}
	if len(out) > MaxStages {
		return Pipeline{}, fmt.Errorf("q: receta supera el máximo de %d etapas", MaxStages)
	}
	p.Stages = out
	return p, nil
}
