// Generate archive metadata from the same inputs used by the release linker.
// Templates in integrations/ are not evidence of an installed/released version.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

func main() {
	version := flag.String("version", "", "release semver without v")
	commit := flag.String("commit", "", "full source SHA")
	buildTime := flag.String("build-time", "", "UTC build timestamp")
	out := flag.String("out", ".goreleaser/metadata", "new metadata directory")
	flag.Parse()
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`).MatchString(*version) ||
		!regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(*commit) {
		fail("release metadata requires exact version and full commit")
	}
	timestamp, err := time.Parse(time.RFC3339, *buildTime)
	if err != nil || timestamp.Location() != time.UTC {
		fail("release metadata requires UTC build time")
	}
	for _, name := range []string{"provider-manifest.json", "tool-card.json"} {
		data, err := os.ReadFile(filepath.Join("integrations", "mi-mcp", name))
		if err != nil {
			fail("metadata template unavailable")
		}
		var obj map[string]any
		if json.Unmarshal(data, &obj) != nil {
			fail("metadata template invalid")
		}
		if name == "provider-manifest.json" {
			obj["provider_version"] = *version
		} else {
			obj["version"] = *version
			obj["provenance"] = map[string]string{
				"commit": *commit, "build_time": *buildTime,
				"source": "https://github.com/fgpaz/mi-lsp/tree/" + *commit,
				"status": "known",
			}
		}
		encoded, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			fail("metadata encoding failed")
		}
		if os.MkdirAll(*out, 0o755) != nil {
			fail("metadata directory unavailable")
		}
		// Never overwrite prior metadata: a new build requires a fresh directory.
		f, err := os.OpenFile(filepath.Join(*out, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			fail("metadata output already exists or is unavailable")
		}
		_, writeErr := f.Write(append(encoded, '\n'))
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			fail("metadata write failed")
		}
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
