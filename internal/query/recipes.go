package query

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed recipes/*.q
var recipeFiles embed.FS

type Recipe struct {
	Name        string
	Version     string
	Description string
	Pipeline    string
}

var recipes = map[string]Recipe{
	"who-calls":       {"who-calls", ContractVersion, "Definición y callers directos", "sym $1 exact | edges callers depth=1 | read ctx=2"},
	"trace":           {"trace", ContractVersion, "Definición, callers y callees", "sym $1 exact | read | edges callers depth=2 | edges callees depth=1"},
	"find-def":        {"find-def", ContractVersion, "Buscar y leer una definición", "sym $1 exact | read"},
	"explain":         {"explain", ContractVersion, "Documentación y símbolos relacionados", "docs $1 | limit 3 | read"},
	"impact":          {"impact", ContractVersion, "Cambio, callers y documentación", "diff ref=$1 | edges callers depth=2 | docs"},
	"explain-change":  {"explain-change", ContractVersion, "Alias de impacto del cambio", "diff ref=$1 | edges callers depth=2 | docs"},
	"nav-intent":      {"nav-intent", ContractVersion, "Ruta de intención documental", "docs $1 | limit 5"},
	"nav-route":       {"nav-route", ContractVersion, "Ruta documental", "docs $1 | limit 1"},
	"nav-pack":        {"nav-pack", ContractVersion, "Paquete documental", "docs $1 | limit 5 | read"},
	"nav-wiki":        {"nav-wiki", ContractVersion, "Búsqueda wiki", "docs $1"},
	"nav-search":      {"nav-search", ContractVersion, "Búsqueda textual", "text $1"},
	"nav-find":        {"nav-find", ContractVersion, "Búsqueda de símbolos", "sym $1"},
	"nav-refs":        {"nav-refs", ContractVersion, "Referencias", "sym $1 exact | edges refs"},
	"nav-related":     {"nav-related", ContractVersion, "Vecindario", "sym $1 exact | edges callers"},
	"nav-flow-slice":  {"nav-flow-slice", ContractVersion, "Flujo de símbolo", "sym $1 exact | edges callers"},
	"nav-change-pack": {"nav-change-pack", ContractVersion, "Impacto de cambio", "diff ref=$1"},
	"nav-affected":    {"nav-affected", ContractVersion, "Superficies afectadas", "diff ref=$1 | edges callers"},
	"nav-multi-read":  {"nav-multi-read", ContractVersion, "Lectura de IDs", "id $1 | read"},
	"nav-overview":    {"nav-overview", ContractVersion, "Resumen de símbolos", "sym $1"},
}

func RecipeList() []Recipe {
	out := make([]Recipe, 0, len(recipes))
	for _, recipe := range recipes {
		out = append(out, recipe)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func Describe(name string) string {
	if name != "" && name[0] == '@' {
		name = name[1:]
	}
	if recipe, ok := recipes[name]; ok {
		return fmt.Sprintf("@%s (%s): %s\nPipeline: %s", recipe.Name, recipe.Version, recipe.Description, recipe.Pipeline)
	}
	return "q-v1: sym, text, docs, id, diff, changed; edges, where, limit, uniq, sort; read, fields, count. Pipeline: stage | stage (máximo 8)."
}

func ExpandRecipe(name string, args []string) ([]Stage, error) {
	name = strings.TrimPrefix(name, "@")
	recipe, ok := recipes[name]
	if !ok {
		return nil, fmt.Errorf("q: receta desconocida @%s", name)
	}
	if len(args) == 0 && (name == "impact" || name == "explain-change" || name == "nav-change-pack" || name == "nav-affected") {
		args = []string{"HEAD"}
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("q: @%s requiere un argumento", name)
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("q: @%s recibe exactamente un argumento", name)
	}
	if strings.Contains(recipe.Pipeline, "$1") {
		recipe.Pipeline = strings.ReplaceAll(recipe.Pipeline, "$1", quoteArg(args[0]))
	}
	parsed, err := ParseWithoutRecipes(recipe.Pipeline)
	if err != nil {
		return nil, err
	}
	return parsed.Stages, nil
}

func ParseWithoutRecipes(s string) (Pipeline, error) {
	// Recipe expansions are trusted source strings, but still parsed by the same grammar.
	p, err := Parse(s)
	if err == nil {
		return p, nil
	}
	if strings.Contains(s, "@") {
		return Pipeline{}, err
	}
	return Pipeline{}, err
}

func quoteArg(s string) string {
	return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(s) + "\""
}

func LoadRecipeSource(name string) (string, error) {
	return recipeFiles.ReadFile("recipes/" + strings.TrimPrefix(name, "@") + ".q")
}
