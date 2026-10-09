package query

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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

const canonicalParseExample = `sym "App.Execute" exact | edges callers depth=2 | read ±3`

type pipelinePart struct {
	text  string
	start int
}

type pipelineToken struct {
	text  string
	start int
}

type parseIssue struct {
	offset   int
	expected string
	cause    error
}

func (e *parseIssue) Error() string {
	if e != nil && e.cause != nil {
		return e.cause.Error()
	}
	return "q syntax error"
}

func Parse(input string) (Pipeline, error) {
	parts, err := splitPipeline(input)
	if err != nil {
		issue, ok := err.(*parseIssue)
		if !ok {
			issue = &parseIssue{expected: "una pipeline q-v1 válida", cause: err}
		}
		return Pipeline{}, formatParseIssue(input, issue)
	}
	if len(parts) == 0 {
		return Pipeline{}, formatParseIssue(input, &parseIssue{expected: "una pipeline de 1 a 8 etapas", cause: fmt.Errorf("pipeline vacía")})
	}
	if len(parts) > MaxStages {
		return Pipeline{}, formatParseIssue(input, &parseIssue{offset: parts[MaxStages].start, expected: fmt.Sprintf("un máximo de %d etapas", MaxStages), cause: fmt.Errorf("q: se admiten de 1 a %d etapas", MaxStages)})
	}
	p := Pipeline{Budget: 2000, MaxBytes: 262144}
	for index, part := range parts {
		tokens, err := tokenize(part.text, part.start)
		if err != nil {
			issue := err.(*parseIssue)
			issue.expected = "comillas cerradas"
			return Pipeline{}, formatParseIssue(input, issue)
		}
		if len(tokens) == 0 {
			return Pipeline{}, formatParseIssue(input, &parseIssue{offset: part.start, expected: "una etapa no vacía", cause: fmt.Errorf("q: etapa %d vacía", index+1)})
		}
		stage := Stage{Verb: strings.ToLower(tokens[0].text), Options: map[string]string{}, Flags: map[string]bool{}}
		var stageArgs []pipelineToken
		for _, token := range tokens[1:] {
			if eq := strings.IndexByte(token.text, '='); eq > 0 {
				key, value := strings.ToLower(token.text[:eq]), token.text[eq+1:]
				if key == "max-bytes" {
					key = "max_bytes"
				}
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
					expected := key + "=<valor válido>"
					if key == "budget" {
						expected = "budget=N, con N entre 1 y 12000"
					} else if key == "max_bytes" {
						expected = "max_bytes=N, con N de al menos 1024"
					}
					return Pipeline{}, formatParseIssue(input, &parseIssue{offset: token.start, expected: expected, cause: fmt.Errorf("q: opción %s inválida", key)})
				}
			} else if token.text == "exact" || token.text == "regex" || token.text == "full" || token.text == "fresh" {
				stage.Flags[token.text] = true
				if token.text == "fresh" {
					p.Fresh = true
				}
			} else {
				stage.Args = append(stage.Args, token.text)
				stageArgs = append(stageArgs, token)
			}
		}
		if stage.Verb == "edges" && len(stage.Args) > 0 {
			switch stage.Args[0] {
			case "ref":
				stage.Args[0] = "refs"
			case "caller":
				stage.Args[0] = "callers"
			case "callee":
				stage.Args[0] = "callees"
			}
		}
		if stage.Verb == "read" {
			if ctx := stage.Options["context"]; ctx != "" && stage.Options["ctx"] == "" {
				stage.Options["ctx"] = ctx
			}
		}
		if err := validateStage(stage); err != nil {
			offset := part.start + len(part.text)
			if len(stageArgs) > 0 {
				offset = stageArgs[len(stageArgs)-1].start
			}
			if strings.Contains(err.Error(), "verbo desconocido") {
				offset = tokens[0].start
			} else if strings.Contains(err.Error(), "requiere") && len(stageArgs) == 0 {
				offset = part.start + len(part.text)
			}
			if strings.Contains(err.Error(), "edges requiere") && len(stageArgs) > 0 {
				offset = stageArgs[0].start
			}
			return Pipeline{}, formatParseIssue(input, &parseIssue{offset: offset, expected: expectedForStageError(stage.Verb, err), cause: fmt.Errorf("q: etapa %d: %w", index+1, err)})
		}
		p.Stages = append(p.Stages, stage)
	}
	if p.Budget < 1 || p.Budget > 12000 {
		return Pipeline{}, formatParseIssue(input, &parseIssue{offset: len(input), expected: "budget=N, con N entre 1 y 12000", cause: fmt.Errorf("q: budget debe estar entre 1 y 12000")})
	}
	if p.MaxBytes < 1024 {
		return Pipeline{}, formatParseIssue(input, &parseIssue{offset: len(input), expected: "max_bytes=N, con N de al menos 1024", cause: fmt.Errorf("q: max_bytes mínimo 1024")})
	}
	if p.Fields != nil {
		for _, field := range p.Fields {
			if !fieldAllowed(field) {
				return Pipeline{}, formatParseIssue(input, &parseIssue{offset: len(input), expected: "un campo permitido: id, rev, kind, name, file, line, text, title", cause: fmt.Errorf("q: campo desconocido %q", field)})
			}
		}
	}
	expanded, err := expandRecipes(p)
	if err != nil {
		return Pipeline{}, formatParseIssue(input, &parseIssue{offset: len(input), expected: "una receta q-v1 existente con su argumento documentado", cause: err})
	}
	return expanded, nil
}

func formatParseIssue(input string, issue *parseIssue) error {
	if issue == nil {
		return fmt.Errorf("q: sintaxis inválida. Ejemplo canónico: %s", canonicalParseExample)
	}
	if issue.expected == "" {
		issue.expected = "una pipeline q-v1 válida"
	}
	if issue.cause == nil {
		issue.cause = fmt.Errorf("sintaxis inválida")
	}
	offset := issue.offset
	if offset < 0 {
		offset = 0
	}
	if offset > len(input) {
		offset = len(input)
	}
	position := utf8.RuneCountInString(input[:offset]) + 1
	return fmt.Errorf("q: posición %d: %v; se esperaba %s. Ejemplo canónico: %s", position, issue.cause, issue.expected, canonicalParseExample)
}

func expectedForStageError(verb string, err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "verbo desconocido"):
		return "un verbo q-v1: sym, text, docs, id, diff, changed, edges, where, limit, uniq, sort, read, fields, count o describe"
	case strings.Contains(message, "edges requiere"):
		return "edges refs|callers|callees|impl"
	case strings.Contains(message, "read espera"):
		return "read ±N, read ctx=N, read full o read file:a-b"
	case strings.Contains(message, "limit requiere"):
		return "limit N, con N entero no negativo"
	case strings.Contains(message, "sort"):
		return "sort <campo> [asc|desc]"
	case strings.Contains(message, "where"):
		return "where <campo><op><valor>, con op =, !=, ~ o !~"
	case strings.Contains(message, "campo"):
		return "un campo permitido para " + verb
	case strings.Contains(message, "requiere argumento") || strings.Contains(message, "requiere al menos"):
		return expectedStageInput(verb)
	case strings.Contains(message, "recibe") || strings.Contains(message, "no recibe"):
		return "la cantidad de argumentos documentada para " + verb
	case strings.Contains(message, "depth"):
		return "depth=1, depth=2 o depth=3"
	case strings.Contains(message, "since"):
		return "changed since=<mark>, con mark entero"
	default:
		return "los argumentos documentados para " + verb
	}
}

func expectedStageInput(verb string) string {
	switch verb {
	case "sym":
		return `sym "<símbolo o glob>"`
	case "text":
		return `text "<patrón>"`
	case "where":
		return "where <campo><op><valor>"
	case "sort":
		return "sort <campo> [asc|desc]"
	case "fields":
		return "fields <campo,campo>"
	case "id":
		return "id <identificador>"
	case "edges":
		return "edges refs|callers|callees|impl"
	case "limit":
		return "limit N"
	default:
		return "un argumento válido para " + verb
	}
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

func splitPipeline(s string) ([]pipelinePart, error) {
	var out []pipelinePart
	start := 0
	quote := rune(0)
	quoteStart := 0
	escaped := false
	appendPart := func(raw string, rawStart int) {
		trimmed := strings.TrimSpace(raw)
		left := len(raw) - len(strings.TrimLeftFunc(raw, unicode.IsSpace))
		out = append(out, pipelinePart{text: trimmed, start: rawStart + left})
	}
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
			quoteStart = i
			continue
		}
		if r == '|' {
			appendPart(s[start:i], start)
			start = i + 1
		}
	}
	if quote != 0 {
		return nil, &parseIssue{offset: quoteStart, expected: "comillas cerradas", cause: fmt.Errorf("comillas sin cerrar")}
	}
	appendPart(s[start:], start)
	return out, nil
}

func tokenize(s string, base int) ([]pipelineToken, error) {
	var tokens []pipelineToken
	var b strings.Builder
	quote := rune(0)
	quoteStart := 0
	escaped := false
	active := false
	tokenStart := 0
	flush := func() {
		if active {
			tokens = append(tokens, pipelineToken{text: b.String(), start: tokenStart})
			b.Reset()
			active = false
		}
	}
	for i, r := range s {
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
			if !active {
				tokenStart = base + i
			}
			quote = r
			quoteStart = base + i
			active = true
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		if !active {
			tokenStart = base + i
		}
		b.WriteRune(r)
		active = true
	}
	if quote != 0 {
		return nil, &parseIssue{offset: quoteStart, expected: "comillas cerradas", cause: fmt.Errorf("comillas sin cerrar")}
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
