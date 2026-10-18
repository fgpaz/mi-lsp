package docgraph

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/wikisource"
)

// WikiIndexEntry is one row from nav wiki index.
type WikiIndexEntry struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Status       string `json:"status,omitempty"`
	Path         string `json:"path"`
	Kind         string `json:"kind"`
	LegacyWarning string `json:"legacy_warning,omitempty"`
}

// WikiIndexResult is the on-demand kind index view.
type WikiIndexResult struct {
	Kind     string           `json:"kind"`
	Count    int              `json:"count"`
	Entries  []WikiIndexEntry `json:"entries"`
	Warnings []string         `json:"warnings,omitempty"`
}

// WikiMatrixCriterionCoverage links one RF criterion to TP cases.
type WikiMatrixCriterionCoverage struct {
	RFID        string                    `json:"rf_id"`
	Criterion   int                       `json:"criterion"`
	Text        string                    `json:"text,omitempty"`
	Covered     bool                      `json:"covered"`
	TestCases   []WikiMatrixTestCaseRef   `json:"test_cases,omitempty"`
}

type WikiMatrixTestCaseRef struct {
	TPID        string `json:"tp_id"`
	Line        string `json:"line,omitempty"`
	Disposition string `json:"disposition,omitempty"`
	Evidence    string `json:"evidence,omitempty"`
}

// WikiMatrixResult is the RF x TP matrix by acceptance criterion.
type WikiMatrixResult struct {
	Schema              string                        `json:"schema"`
	RFsReviewed         int                           `json:"rfs_reviewed"`
	TPsReviewed         int                           `json:"tps_reviewed"`
	Criteria            []WikiMatrixCriterionCoverage `json:"criteria"`
	UncoveredCriteria   []WikiMatrixCriterionCoverage `json:"uncovered_criteria,omitempty"`
	TPsWithoutRF        []string                      `json:"tps_without_rf,omitempty"`
	Warnings            []string                      `json:"warnings,omitempty"`
}

// WikiViewDoc is minimal input for index/matrix builders.
type WikiViewDoc struct {
	Path    string
	DocID   string
	Title   string
	Content string
}

// BuildWikiIndex lists documents for a v1 kind across legacy and v1 layouts.
func BuildWikiIndex(docs []WikiViewDoc, requestedKind string) WikiIndexResult {
	kind, ok := NormalizeWikiKind(requestedKind)
	result := WikiIndexResult{Kind: requestedKind, Entries: []WikiIndexEntry{}}
	if !ok {
		result.Warnings = append(result.Warnings, "unknown_kind: use requirement, test, flow, outcome, technical, ... or RF, TP, FL, RS")
		return result
	}
	result.Kind = kind
	entries := make([]WikiIndexEntry, 0)
	seen := map[string]struct{}{}
	for _, doc := range docs {
		meta := wikisource.ParseContractMetadata(doc.Content)
		declaredKind := meta.Kind
		if !DocMatchesWikiKind(doc.Path, firstNonEmpty(meta.ID, doc.DocID), declaredKind, kind) {
			continue
		}
		id := firstNonEmpty(meta.ID, doc.DocID)
		if id == "" {
			id = filepath.Base(doc.Path)
		}
		key := strings.ToLower(id) + "\x00" + strings.ToLower(doc.Path)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		title := firstNonEmpty(meta.Title, doc.Title, id)
		entry := WikiIndexEntry{
			ID:     id,
			Title:  title,
			Status: meta.Status,
			Path:   filepath.ToSlash(doc.Path),
			Kind:   kind,
		}
		if warn := LegacyPathRenameWarning(doc.Path); warn != "" {
			entry.LegacyWarning = warn
			result.Warnings = appendUniqueString(result.Warnings, warn)
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].ID) < strings.ToLower(entries[j].ID)
	})
	result.Entries = entries
	result.Count = len(entries)
	if result.Count == 0 {
		result.Warnings = appendUniqueString(result.Warnings, "wiki_index_empty: no documents matched kind "+kind)
	}
	return result
}

// BuildWikiMatrix computes RF x TP coverage from verifies links and case lines.
func BuildWikiMatrix(docs []WikiViewDoc) WikiMatrixResult {
	result := WikiMatrixResult{
		Schema:     "wiki-matrix/v1",
		Criteria:   []WikiMatrixCriterionCoverage{},
		Warnings:   []string{},
	}
	rfDocs := map[string]WikiViewDoc{}
	tpDocs := map[string]WikiViewDoc{}
	for _, doc := range docs {
		meta := wikisource.ParseContractMetadata(doc.Content)
		id := firstNonEmpty(meta.ID, doc.DocID)
		kind := WikiContractKindForPath(doc.Path, meta.Kind)
		if kind == WikiKindRequirement || strings.HasPrefix(strings.ToUpper(id), "RF-") {
			if id != "" {
				rfDocs[strings.ToUpper(id)] = doc
			}
			continue
		}
		if kind == WikiKindTest || strings.HasPrefix(strings.ToUpper(id), "TP-") {
			if id != "" {
				tpDocs[strings.ToUpper(id)] = doc
			}
		}
	}
	result.RFsReviewed = len(rfDocs)
	result.TPsReviewed = len(tpDocs)

	coverage := map[string]map[int][]WikiMatrixTestCaseRef{}
	for tpID, doc := range tpDocs {
		meta := wikisource.ParseContractMetadata(doc.Content)
		rfTargets := meta.Links.Verifies
		if len(rfTargets) == 0 {
			rfTargets = wikisource.LegacyVerifiesFromImports(doc.Content)
		}
		if len(rfTargets) == 0 {
			result.TPsWithoutRF = appendUniqueString(result.TPsWithoutRF, tpID)
			result.Warnings = appendUniqueString(result.Warnings, "tp_without_rf:"+tpID)
			continue
		}
		cases := wikisource.ParseTestPlanCases(doc.Content)
		if len(cases) == 0 {
			result.Warnings = appendUniqueString(result.Warnings, "tp_without_cases:"+tpID)
		}
		for _, rfID := range rfTargets {
			rfKey := strings.ToUpper(strings.TrimSpace(rfID))
			if rfKey == "" {
				continue
			}
			if coverage[rfKey] == nil {
				coverage[rfKey] = map[int][]WikiMatrixTestCaseRef{}
			}
			for _, tc := range cases {
				coverage[rfKey][tc.Criterion] = append(coverage[rfKey][tc.Criterion], WikiMatrixTestCaseRef{
					TPID:        tpID,
					Line:        tc.Line,
					Disposition: tc.Disposition,
					Evidence:    tc.Evidence,
				})
			}
		}
	}
	sort.Strings(result.TPsWithoutRF)

	allCriteria := make([]WikiMatrixCriterionCoverage, 0)
	for rfID, doc := range rfDocs {
		criteria := wikisource.ParseAcceptanceCriteria(doc.Content)
		if len(criteria) == 0 {
			result.Warnings = appendUniqueString(result.Warnings, "rf_missing_acceptance_criteria:"+rfID)
			continue
		}
		nums := make([]int, 0, len(criteria))
		for n := range criteria {
			nums = append(nums, n)
		}
		sort.Ints(nums)
		for _, n := range nums {
			tests := coverage[rfID][n]
			row := WikiMatrixCriterionCoverage{
				RFID:      rfID,
				Criterion: n,
				Text:      criteria[n],
				Covered:   len(tests) > 0,
				TestCases: tests,
			}
			allCriteria = append(allCriteria, row)
			if !row.Covered {
				result.UncoveredCriteria = append(result.UncoveredCriteria, row)
			}
		}
	}
	sort.Slice(allCriteria, func(i, j int) bool {
		a, b := allCriteria[i], allCriteria[j]
		if a.RFID != b.RFID {
			return a.RFID < b.RFID
		}
		return a.Criterion < b.Criterion
	})
	result.Criteria = allCriteria
	if len(result.UncoveredCriteria) > 0 {
		result.Warnings = appendUniqueString(result.Warnings, "uncovered_acceptance_criteria:"+itoa(len(result.UncoveredCriteria)))
	}
	return result
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 12)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
