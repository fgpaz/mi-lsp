package service

import (
	"strings"
	"testing"
)

func legacyNormalizeRankingText(value string) string {
	value = strings.ToLower(value)
	replacer := strings.NewReplacer(
		"\r", " ", "\n", " ", "_", " ", "-", " ", "/", " ", "\\", " ", ".", " ",
		":", " ", ",", " ", ";", " ", "?", " ", "!", " ", "(", " ", ")", " ",
	)
	value = replacer.Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func TestNormalizeRankingTextMatchesLegacy(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"RF-GPH-010",
		"  Hola_Mundo/path\\to.file:42, ¿Qué? ¡Sí! (NAVEGACIÓN)  ",
		"line1\r\nline2\tTAB\u00a0nbsp",
		"ÀÉÎÕÜ ñandú İstanbul ΣΑΣ",
		"a--b__c..d",
		"--leading and trailing--",
	}
	for _, input := range cases {
		if got, want := normalizeRankingText(input), legacyNormalizeRankingText(input); got != want {
			t.Errorf("normalizeRankingText(%q) = %q, want %q", input, got, want)
		}
	}
}

func BenchmarkNormalizeRankingText(b *testing.B) {
	text := strings.Repeat("Mi-LSP navigation: RF_GPH-010 (route) / Wiki.Docs; ", 200)
	for i := 0; i < b.N; i++ {
		normalizeRankingText(text)
	}
}

func BenchmarkLegacyNormalizeRankingText(b *testing.B) {
	text := strings.Repeat("Mi-LSP navigation: RF_GPH-010 (route) / Wiki.Docs; ", 200)
	for i := 0; i < b.N; i++ {
		legacyNormalizeRankingText(text)
	}
}
