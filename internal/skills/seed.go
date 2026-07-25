package skills

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
)

//go:embed seed_catalog.csv
var seedCatalogCSV []byte

// LoadEmbeddedSeed parses the embedded research seed CSV.
func LoadEmbeddedSeed() ([]SeedRow, error) {
	return parseSeedCSV(bytes.NewReader(seedCatalogCSV))
}

// LoadSeedFile parses a seed CSV from disk.
func LoadSeedFile(path string) ([]SeedRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseSeedCSV(f)
}

// SeedMap indexes seed rows by skill id.
func SeedMap(rows []SeedRow) map[string]SeedRow {
	out := make(map[string]SeedRow, len(rows))
	for _, r := range rows {
		out[r.ID] = r
	}
	return out
}

func parseSeedCSV(r io.Reader) ([]SeedRow, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	cr.FieldsPerRecord = -1

	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("seed csv header: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(strings.ToLower(h))] = i
	}
	required := []string{"id", "family", "suggested_tier", "suggested_audience", "suggested_aliases", "critical_candidate"}
	for _, name := range required {
		if _, ok := col[name]; !ok {
			return nil, fmt.Errorf("seed csv missing column %q", name)
		}
	}

	var rows []SeedRow
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("seed csv row: %w", err)
		}
		id := strings.TrimSpace(fieldAt(rec, col["id"]))
		if id == "" {
			continue
		}
		aliasesRaw := strings.TrimSpace(fieldAt(rec, col["suggested_aliases"]))
		var aliases []string
		if aliasesRaw != "" {
			for _, a := range strings.Split(aliasesRaw, "|") {
				a = strings.TrimSpace(a)
				if a != "" {
					aliases = append(aliases, a)
				}
			}
			// also accept comma-separated if single field had commas (rare)
			if len(aliases) == 1 && strings.Contains(aliases[0], ",") {
				parts := strings.Split(aliases[0], ",")
				aliases = aliases[:0]
				for _, a := range parts {
					a = strings.TrimSpace(a)
					if a != "" {
						aliases = append(aliases, a)
					}
				}
			}
		}
		crit := strings.EqualFold(strings.TrimSpace(fieldAt(rec, col["critical_candidate"])), "true")
		rows = append(rows, SeedRow{
			ID:                id,
			Family:            strings.TrimSpace(fieldAt(rec, col["family"])),
			SuggestedTier:     strings.TrimSpace(fieldAt(rec, col["suggested_tier"])),
			SuggestedAudience: strings.TrimSpace(fieldAt(rec, col["suggested_audience"])),
			SuggestedAliases:  aliases,
			CriticalCandidate: crit,
		})
	}
	return rows, nil
}

func fieldAt(rec []string, i int) string {
	if i < 0 || i >= len(rec) {
		return ""
	}
	return rec[i]
}
