// Package grepx implements `mi-lsp grep` (contract grep-v1): a transparent
// passthrough to the real ripgrep that, when its stdout goes to an agent,
// appends a per-line annotation `\t⟦<def|ref|com|str> <container>⟧` to match
// lines using the read-only catalog. It never fails where rg does not fail.
package grepx

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultBudget is the annotation time budget (grep-v1).
const DefaultBudget = 300 * time.Millisecond

// NoIndexNotice is the single stderr line printed when matches fell in code
// files but no catalog was available.
const NoIndexNotice = "mi-lsp: sin índice (index_not_ready)"

// Options configures one grep run.
type Options struct {
	Args     []string // rg arguments, verbatim (own flags already removed)
	RgCompat bool     // pure passthrough, no annotation
	Annotate bool     // force annotation even when stdout is not a pipe
	Stdin    io.Reader
	Stdout   *os.File
	Stderr   io.Writer
	Cwd      string
	Getenv   func(string) string
	Budget   time.Duration
}

// Result summarizes a run for telemetry. It never holds content.
type Result struct {
	ExitCode  int
	BytesOut  int64
	Matches   int
	Annotated int
	Mode      string // raw | annotated | no_index
	Workspace string
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// Run executes rg and, when eligible, annotates its output.
func Run(ctx context.Context, opts Options) Result {
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Cwd == "" {
		opts.Cwd, _ = os.Getwd()
	}
	if opts.Budget <= 0 {
		opts.Budget = DefaultBudget
	}

	rgPath, err := locateRg(opts.Getenv)
	if err != nil {
		fmt.Fprintln(opts.Stderr, "mi-lsp: rg no encontrado (definir MI_LSP_RG o agregar rg al PATH)")
		return Result{ExitCode: 2, Mode: "raw"}
	}

	info := analyzeArgs(opts.Args)
	if !shouldAnnotate(opts, info) {
		return runRaw(ctx, rgPath, opts)
	}
	return runAnnotated(ctx, rgPath, opts, info)
}

func locateRg(getenv func(string) string) (string, error) {
	if custom := strings.TrimSpace(getenv("MI_LSP_RG")); custom != "" {
		return custom, nil
	}
	return exec.LookPath("rg")
}

func shouldAnnotate(opts Options, info argInfo) bool {
	if opts.RgCompat || info.skipAnnotate {
		return false
	}
	if opts.Getenv("MI_LSP_GREP_ANNOTATE") == "0" || opts.Getenv("RIPGREP_CONFIG_PATH") != "" {
		return false
	}
	if opts.Annotate {
		return true
	}
	return detectStdout(opts.Stdout) == stdoutPipe
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
	}
	return 2
}

// runRaw gives rg the real stdin/stdout/stderr file descriptors, so output and
// exit status are exactly rg's.
func runRaw(ctx context.Context, rgPath string, opts Options) Result {
	cmd := exec.CommandContext(ctx, rgPath, opts.Args...)
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Dir = opts.Cwd
	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		fmt.Fprintf(opts.Stderr, "mi-lsp: no se pudo ejecutar rg: %v\n", err)
		return Result{ExitCode: 2, Mode: "raw"}
	}
	return Result{ExitCode: exitCode(err), Mode: "raw"}
}

// injectFlags adds the flags that make rg's output parseable: line numbers,
// NUL after the path, no color, no heading. They go before a `--` terminator.
func injectFlags(args []string, terminator int) []string {
	extra := []string{"-n", "-0", "--color=never", "--no-heading"}
	out := make([]string, 0, len(args)+len(extra))
	if terminator < 0 {
		out = append(out, args...)
		return append(out, extra...)
	}
	out = append(out, args[:terminator]...)
	out = append(out, extra...)
	return append(out, args[terminator:]...)
}

// searchStart picks the directory used to find the catalog: the first path
// argument, else the cwd.
func searchStart(info argInfo, cwd string) string {
	if len(info.paths) > 0 && info.paths[0] != "-" {
		path := info.paths[0]
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		return path
	}
	return cwd
}

// runAnnotated runs rg with parseable flags, rebuilds the exact output the
// user's flags would have produced, and annotates match lines. Any failure in
// the annotation layer degrades to unannotated lines; rg's status is final.
func runAnnotated(ctx context.Context, rgPath string, opts Options, info argInfo) Result {
	cmd := exec.CommandContext(ctx, rgPath, injectFlags(opts.Args, info.terminator)...)
	cmd.Stdin = opts.Stdin
	cmd.Stderr = opts.Stderr
	cmd.Dir = opts.Cwd
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return runRaw(ctx, rgPath, opts)
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(opts.Stderr, "mi-lsp: no se pudo ejecutar rg: %v\n", err)
		return Result{ExitCode: 2, Mode: "raw"}
	}

	annotationCtx, cancelAnnotation := context.WithTimeout(ctx, opts.Budget)
	defer cancelAnnotation()
	proc := &processor{
		cwd:    opts.Cwd,
		info:   info,
		m:      newMatcher(info),
		budget: opts.Budget,
		ctx:    annotationCtx,
	}
	result := Result{Mode: "annotated"}
	if root, ok := resolveCatalogRoot(searchStart(info, opts.Cwd)); ok {
		proc.cat = newCatalog(root)
		result.Workspace = root
		defer proc.cat.close()
	}

	counter := &countingWriter{w: opts.Stdout}
	writer := bufio.NewWriterSize(counter, 64*1024)
	copyErr := proc.run(pipe, writer)
	if copyErr != nil {
		// stdout is gone (closed pipe): stop rg and report it like rg would.
		_ = cmd.Process.Kill()
		_, _ = io.Copy(io.Discard, pipe)
	}
	_ = writer.Flush()
	waitErr := cmd.Wait()

	result.ExitCode = exitCode(waitErr)
	result.BytesOut = counter.n
	result.Matches = proc.matches
	result.Annotated = proc.annotated
	if proc.sawCode && (proc.cat == nil || proc.cat.err != nil) {
		fmt.Fprintln(opts.Stderr, NoIndexNotice)
		result.Mode = "no_index"
	}
	return result
}
