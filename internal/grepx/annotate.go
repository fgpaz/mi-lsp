package grepx

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/fgpaz/mi-lsp/internal/language"
)

// lineRecord is one parsed line of the enriched rg output (rg run with -n -0).
type lineRecord struct {
	path    []byte // nil when rg prints no filename (single file / stdin)
	number  int
	match   bool // ':' separator (match line); false means context ('-')
	content []byte
}

// parseEnriched parses `path\0N:content`, `path\0N-content`, `N:content` or
// `N-content`. ok is false for anything else (separators, notices).
func parseEnriched(line []byte) (lineRecord, bool) {
	var rec lineRecord
	rest := line
	if idx := bytes.IndexByte(line, 0); idx >= 0 {
		rec.path = line[:idx]
		rest = line[idx+1:]
	}
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits >= len(rest) || digits > 9 {
		return lineRecord{}, false
	}
	sep := rest[digits]
	if sep != ':' && sep != '-' {
		return lineRecord{}, false
	}
	number, err := strconv.Atoi(string(rest[:digits]))
	if err != nil {
		return lineRecord{}, false
	}
	rec.number = number
	rec.match = sep == ':'
	rec.content = rest[digits+1:]
	return rec, true
}

// render rebuilds the line exactly as rg would have printed it for the user's
// own flags: path, optional line number, content. suffix is appended to the
// content (before a trailing CR) for annotated match lines.
func render(dst []byte, rec lineRecord, userLineNumber bool, suffix string) []byte {
	sep := byte('-')
	if rec.match {
		sep = ':'
	}
	if rec.path != nil {
		dst = append(dst, rec.path...)
		dst = append(dst, sep)
	}
	if userLineNumber {
		dst = strconv.AppendInt(dst, int64(rec.number), 10)
		dst = append(dst, sep)
	}
	content := rec.content
	if suffix == "" {
		return append(dst, content...)
	}
	cr := false
	if n := len(content); n > 0 && content[n-1] == '\r' {
		content = content[:n-1]
		cr = true
	}
	dst = append(dst, content...)
	dst = append(dst, suffix...)
	if cr {
		dst = append(dst, '\r')
	}
	return dst
}

// matcher finds the first match column of the user's patterns in a line and
// holds the literals used for def detection.
type matcher struct {
	re       *regexp.Regexp
	literals []string
	fold     bool
}

func newMatcher(info argInfo) *matcher {
	m := &matcher{}
	if info.patternFile || info.pcre || len(info.patterns) == 0 {
		return m
	}
	fold := info.ignoreCase
	if info.smartCase && !info.ignoreCase {
		fold = true
		for _, p := range info.patterns {
			for _, r := range p {
				if unicode.IsUpper(r) {
					fold = false
				}
			}
		}
	}
	m.fold = fold
	parts := make([]string, 0, len(info.patterns))
	for _, pattern := range info.patterns {
		body := pattern
		if info.fixed {
			body = regexp.QuoteMeta(pattern)
			if lit := strings.TrimSpace(pattern); len(lit) >= 2 {
				m.literals = append(m.literals, lit)
			}
		} else if lit := longestIdentifier(pattern); len(lit) >= 2 {
			m.literals = append(m.literals, lit)
		}
		if info.word {
			body = `\b(?:` + body + `)\b`
		}
		if info.line {
			body = `^(?:` + body + `)$`
		}
		parts = append(parts, "(?:"+body+")")
	}
	expr := strings.Join(parts, "|")
	if fold {
		expr = "(?i)" + expr
	}
	if re, err := regexp.Compile(expr); err == nil {
		m.re = re
	}
	return m
}

func longestIdentifier(pattern string) string {
	best, start := "", -1
	flush := func(end int) {
		if start >= 0 && end-start > len(best) {
			best = pattern[start:end]
		}
		start = -1
	}
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		isID := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if isID {
			if start < 0 {
				start = i
			}
			continue
		}
		// An escaped character (\w, \d, \() is not part of an identifier run.
		if c == '\\' && i+1 < len(pattern) {
			flush(i)
			i++
			continue
		}
		flush(i)
	}
	flush(len(pattern))
	return best
}

// offset returns the byte offset of the first match, or -1 when unknown.
func (m *matcher) offset(content string) int {
	if m.re == nil {
		return -1
	}
	loc := m.re.FindStringIndex(content)
	if loc == nil {
		return -1
	}
	return loc[0]
}

func (m *matcher) nameHasLiteral(name string) bool {
	if len(m.literals) == 0 {
		return false
	}
	if m.fold {
		name = strings.ToLower(name)
	}
	for _, lit := range m.literals {
		if m.fold {
			lit = strings.ToLower(lit)
		}
		if strings.Contains(name, lit) {
			return true
		}
	}
	return false
}

// processor streams enriched rg output to the user's stdout.
type processor struct {
	cwd        string
	info       argInfo
	cat        *catalog
	m          *matcher
	budget     time.Duration
	spent      time.Duration
	disabled   bool
	ctx        context.Context
	matches    int
	annotated  int
	sawCode    bool // a match fell in a code file
	hadCatalog bool
}

func (p *processor) run(r io.Reader, w *bufio.Writer) error {
	reader := bufio.NewReaderSize(r, 256*1024)
	var out []byte
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			out = p.handle(out[:0], line)
			if _, werr := w.Write(out); werr != nil {
				return werr
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// handle converts one enriched line to its output form.
func (p *processor) handle(dst []byte, line []byte) []byte {
	body, newline := line, byte(0)
	if n := len(body); n > 0 && body[n-1] == '\n' {
		body, newline = body[:n-1], '\n'
	}
	rec, ok := parseEnriched(body)
	if !ok {
		return append(dst, line...)
	}
	suffix := ""
	if rec.match {
		p.matches++
		suffix = p.annotate(rec)
	}
	dst = render(dst, rec, p.info.lineNumber, suffix)
	if newline != 0 {
		dst = append(dst, newline)
	}
	return dst
}

func (p *processor) annotate(rec lineRecord) (suffix string) {
	if rec.path == nil {
		return ""
	}
	path := string(rec.path)
	lang, isCode := language.ForPath(path)
	if !isCode {
		return ""
	}
	p.sawCode = true
	if p.cat == nil || p.disabled {
		return ""
	}
	started := time.Now()
	defer func() {
		if recover() != nil {
			p.disabled = true
			suffix = ""
		}
		p.spent += time.Since(started)
		if p.spent > p.budget {
			p.disabled = true
		}
	}()
	rel, ok := p.cat.relativePath(p.cwd, path)
	if !ok {
		return ""
	}
	file, err := p.cat.lookup(p.ctx, rel)
	if err != nil {
		p.disabled = true
		return ""
	}
	if !file.indexed {
		return ""
	}
	p.hadCatalog = true
	content := string(rec.content)
	if n := len(content); n > 0 && content[n-1] == '\r' {
		content = content[:n-1]
	}
	class, container := "", ""
	for _, sym := range file.startingAt(rec.number) {
		if p.m.nameHasLiteral(sym.name) && strings.Contains(content, sym.name) {
			class, container = classDef, sym.qualified
			break
		}
	}
	if class == "" {
		class = classifyOffset(lang, content, p.m.offset(content))
		if sym, ok := file.containing(rec.number); ok {
			container = sym.qualified
		}
	}
	p.annotated++
	if container == "" {
		return "\t⟦" + class + "⟧"
	}
	return "\t⟦" + class + " " + container + "⟧"
}

// stdoutKind classifies where the process stdout goes.
type stdoutKind int

const (
	stdoutPipe stdoutKind = iota // pipe or socket: how agent harnesses capture output
	stdoutTTY                    // terminal or other character device
	stdoutFile                   // regular file redirection
)

func detectStdout(f *os.File) stdoutKind {
	if f == nil {
		return stdoutPipe
	}
	info, err := f.Stat()
	if err != nil {
		return stdoutPipe
	}
	mode := info.Mode()
	switch {
	case mode&os.ModeCharDevice != 0:
		return stdoutTTY
	case mode.IsRegular():
		return stdoutFile
	default:
		return stdoutPipe
	}
}
