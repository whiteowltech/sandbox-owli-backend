// Command importfoods arma el catálogo de alimentos a partir de USDA FoodData
// Central y de dos archivos curados:
//   - data/foods_catalog.csv: alimentos de USDA, con nombres en español e inglés,
//     sinónimos y atributos de dieta.
//   - data/recipes_catalog.csv: platos que USDA no tiene (chimichurri, empanadas…),
//     calculados con domain.ComposeRecipe a partir de ingredientes de USDA.
//
// Uso (desde backend/):
//
//	go run ./cmd/importfoods -search "chicken breast roasted; rice white cooked"
//	go run ./cmd/importfoods                  # escribe data/foods_seed.json (embebido en la API)
//
// Los JSON de USDA se descargan en .fdc-cache/ (ver README de la sección Comandos).
package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/whiteowltech/owli/backend/internal/adapters/fdc"
	"github.com/whiteowltech/owli/backend/internal/catalog"
	"github.com/whiteowltech/owli/backend/internal/domain"
)

const (
	foundationFile = ".fdc-cache/FoodData_Central_foundation_food_json_2026-10-29.json"
	srLegacyFile   = ".fdc-cache/FoodData_Central_sr_legacy_food_json_2018-04.json"
	catalogFile    = "data/foods_catalog.csv"
	recipesFile    = "data/recipes_catalog.csv"
	seedFile       = "data/foods_seed.json"
)

func main() {
	search := flag.String("search", "", "consultas en inglés separadas por ';' para elegir fichas de USDA")
	dryRun := flag.Bool("dry-run", false, "valida el catálogo sin escribir en la base")
	flag.Parse()
	if err := run(*search, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(search string, dryRun bool) error {
	db, err := loadUSDA()
	if err != nil {
		return err
	}
	if search != "" {
		for _, q := range strings.Split(search, ";") {
			q = strings.TrimSpace(q)
			fmt.Printf("\n# %s\n", q)
			for _, r := range fdc.Search(db, q, 6) {
				m, _ := r.Macros()
				fmt.Printf("%d\t%-10s\t%4.0f kcal  P%4.1f C%4.1f G%4.1f\t%s\n", r.FdcID, r.Dataset, m.Calories, m.ProteinG, m.CarbsG, m.FatG, r.Description)
			}
		}
		return nil
	}

	foods, err := readCatalog(catalogFile, db)
	if err != nil {
		return err
	}
	recipes, err := readRecipes(recipesFile, db)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, f := range foods {
		seen[f.ID] = true
	}
	for _, r := range recipes {
		if seen[r.ID] {
			return fmt.Errorf("la receta %q repite un id del catálogo", r.ID)
		}
	}
	foods = append(foods, recipes...)
	fmt.Printf("catálogo válido: %d alimentos (%d recetas)\n", len(foods), len(recipes))
	if dryRun {
		return nil
	}
	raw, err := json.MarshalIndent(foods, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(seedFile, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("escrito %s: la API lo carga al arrancar\n", seedFile)
	return nil
}

func loadUSDA() (map[int]fdc.Record, error) {
	all := map[int]fdc.Record{}
	for _, src := range []struct{ path, name string }{{foundationFile, "foundation"}, {srLegacyFile, "sr_legacy"}} {
		recs, err := fdc.Load(src.path, src.name)
		if err != nil {
			return nil, fmt.Errorf("%w (¿descargaste los datos de USDA en .fdc-cache/?)", err)
		}
		for id, r := range recs {
			all[id] = r
		}
	}
	return all, nil
}

// readCatalog lee el catálogo curado. Columnas:
// id, fdc_id, name_es, name_en, aliases (separados por |), traits (separados por |)
// traits válidos: animal, plant, meat, fish, grain, legume, dairy, added_sugar, gluten, lactose, verified
func readCatalog(path string, db map[int]fdc.Record) ([]catalog.Food, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("leyendo encabezado: %w", err)
	}
	if strings.Join(header, ",") != "id,fdc_id,name_es,name_en,aliases,traits" {
		return nil, fmt.Errorf("encabezado inesperado: %v", header)
	}
	seen := map[string]bool{}
	var out []catalog.Food
	var problems []string
	for line := 2; ; line++ {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("línea %d: %w", line, err)
		}
		id, fdcStr, nameES, nameEN, aliases, traitsStr := row[0], row[1], row[2], row[3], row[4], row[5]
		if seen[id] {
			problems = append(problems, fmt.Sprintf("línea %d: id repetido %q", line, id))
			continue
		}
		seen[id] = true
		fdcID, err := strconv.Atoi(fdcStr)
		if err != nil {
			problems = append(problems, fmt.Sprintf("línea %d: fdc_id %q", line, fdcStr))
			continue
		}
		rec, ok := db[fdcID]
		if !ok {
			problems = append(problems, fmt.Sprintf("línea %d (%s): fdc_id %d no existe en USDA", line, id, fdcID))
			continue
		}
		macros, err := rec.Macros()
		if err != nil {
			problems = append(problems, fmt.Sprintf("línea %d (%s): %v", line, id, err))
			continue
		}
		traits, err := parseTraits(traitsStr)
		if err != nil {
			problems = append(problems, fmt.Sprintf("línea %d (%s): %v", line, id, err))
			continue
		}
		out = append(out, catalog.FromDomain(domain.Food{
			ID: domain.FoodID(id), NameES: nameES, NameEN: nameEN,
			Per100g: macros, Micros: rec.Micros(), Traits: traits,
		}, splitPipe(aliases), "usda_"+rec.Dataset, strconv.Itoa(fdcID)))
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("el catálogo tiene %d problemas:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	return out, nil
}

// readRecipes lee las recetas compuestas. Columnas:
// id, name_es, name_en, aliases, traits, yield_g, ingredients
// ingredients: "fdc_id:gramos|fdc_id:gramos…"; yield_g vacío = suma de ingredientes.
// Un plato nunca lleva "verified": sus alérgenos dependen de cada preparación.
func readRecipes(path string, db map[int]fdc.Record) ([]catalog.Food, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("leyendo encabezado de recetas: %w", err)
	}
	if strings.Join(header, ",") != "id,name_es,name_en,aliases,traits,yield_g,ingredients" {
		return nil, fmt.Errorf("encabezado de recetas inesperado: %v", header)
	}
	seen := map[string]bool{}
	var out []catalog.Food
	var problems []string
	for line := 2; ; line++ {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("recetas, línea %d: %w", line, err)
		}
		id, nameES, nameEN, aliases, traitsStr, yieldStr, ingredients := row[0], row[1], row[2], row[3], row[4], row[5], row[6]
		fail := func(msg string, args ...any) {
			problems = append(problems, fmt.Sprintf("receta %s (línea %d): ", id, line)+fmt.Sprintf(msg, args...))
		}
		if seen[id] {
			fail("id repetido")
			continue
		}
		seen[id] = true
		traits, err := parseTraits(traitsStr)
		if err != nil {
			fail("%v", err)
			continue
		}
		if traits.AllergensVerified {
			fail("un plato no puede tener los alérgenos verificados")
			continue
		}
		yieldG := 0.0
		if yieldStr != "" {
			if yieldG, err = strconv.ParseFloat(yieldStr, 64); err != nil {
				fail("yield_g %q", yieldStr)
				continue
			}
		}
		parts, err := recipeParts(ingredients, db)
		if err != nil {
			fail("%v", err)
			continue
		}
		macros, micros, err := domain.ComposeRecipe(parts, yieldG)
		if err != nil {
			fail("%v", err)
			continue
		}
		out = append(out, catalog.FromDomain(domain.Food{
			ID: domain.FoodID(id), NameES: nameES, NameEN: nameEN,
			Per100g: macros, Micros: micros, Traits: traits,
		}, splitPipe(aliases), "owli_recipe", ingredients))
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("las recetas tienen %d problemas:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	return out, nil
}

func recipeParts(s string, db map[int]fdc.Record) ([]domain.RecipePart, error) {
	var parts []domain.RecipePart
	for _, item := range splitPipe(s) {
		idStr, gStr, ok := strings.Cut(item, ":")
		if !ok {
			return nil, fmt.Errorf("ingrediente %q: falta \":gramos\"", item)
		}
		fdcID, err := strconv.Atoi(idStr)
		if err != nil {
			return nil, fmt.Errorf("ingrediente %q: fdc_id inválido", item)
		}
		grams, err := strconv.ParseFloat(gStr, 64)
		if err != nil {
			return nil, fmt.Errorf("ingrediente %q: gramos inválidos", item)
		}
		rec, ok := db[fdcID]
		if !ok {
			return nil, fmt.Errorf("ingrediente %d no existe en USDA", fdcID)
		}
		m, err := rec.Macros()
		if err != nil {
			return nil, err
		}
		parts = append(parts, domain.RecipePart{Per100g: m, Micros: rec.Micros(), Grams: grams})
	}
	return parts, nil
}

func splitPipe(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "|") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseTraits(s string) (domain.FoodTraits, error) {
	var t domain.FoodTraits
	for _, x := range splitPipe(s) {
		switch x {
		case "animal":
			t.AnimalOrigin = true
		case "plant":
			t.PlantOrigin = true
		case "meat":
			t.ContainsMeat = true
		case "fish":
			t.ContainsFish = true
		case "grain":
			t.ContainsGrain = true
		case "legume":
			t.ContainsLegume = true
		case "dairy":
			t.ContainsDairy = true
		case "added_sugar":
			t.ContainsAddedSugar = true
		case "gluten":
			t.ContainsGluten = true
		case "lactose":
			t.ContainsLactose = true
		case "verified":
			t.AllergensVerified = true
		default:
			return t, fmt.Errorf("atributo desconocido %q", x)
		}
	}
	if !t.AnimalOrigin && !t.PlantOrigin && (t.ContainsMeat || t.ContainsFish || t.ContainsDairy) {
		return t, errors.New("carne, pescado o lácteo implican origen animal")
	}
	return t, nil
}
