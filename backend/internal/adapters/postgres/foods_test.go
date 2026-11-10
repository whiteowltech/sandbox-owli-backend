package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/whiteowltech/owli/backend/data"
	"github.com/whiteowltech/owli/backend/internal/catalog"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

func ptr(v float64) *float64 { return &v }

func seedFoods(t *testing.T, s *Store) {
	t.Helper()
	foods := []CatalogFood{
		{Food: domain.Food{ID: "pechuga-pollo", NameES: "Pechuga de pollo asada", NameEN: "Roasted chicken breast",
			Per100g: domain.Macros{Calories: 165, ProteinG: 31, FatG: 3.57}, Micros: domain.Micros{PotassiumMg: ptr(256)},
			Traits: domain.FoodTraits{AnimalOrigin: true, ContainsMeat: true, AllergensVerified: true}},
			Aliases: []string{"pollo", "pechuga"}, Source: "usda_sr_legacy", SourceRef: "171477"},
		{Food: domain.Food{ID: "palta", NameES: "Palta", NameEN: "Avocado",
			Per100g: domain.Macros{Calories: 160, ProteinG: 2, CarbsG: 8.53, FiberG: 6.7, FatG: 14.7},
			Traits:  domain.FoodTraits{PlantOrigin: true, AllergensVerified: true}},
			Aliases: []string{"aguacate"}, Source: "usda_sr_legacy", SourceRef: "171705"},
		{Food: domain.Food{ID: "jugo-limon", NameES: "Jugo de limón", NameEN: "Lemon juice",
			Per100g: domain.Macros{Calories: 22, ProteinG: 0.35, CarbsG: 6.9, FiberG: 0.3, FatG: 0.24}},
			Source: "usda_sr_legacy", SourceRef: "167747"},
	}
	if err := s.UpsertFoods(context.Background(), foods); err != nil {
		t.Fatalf("UpsertFoods: %v", err)
	}
}

func TestFoodSearch(t *testing.T) {
	s, pool := testStore(t)
	if _, err := pool.Exec(context.Background(), "TRUNCATE foods CASCADE"); err != nil {
		t.Fatal(err)
	}
	seedFoods(t, s)
	seedFoods(t, s) // importar dos veces es idempotente
	ctx := context.Background()
	foods := s.Foods()

	tests := map[string]domain.FoodID{
		"pollo":    "pechuga-pollo", // sinónimo
		"Pechuga":  "pechuga-pollo",
		"pechga":   "pechuga-pollo", // error de tipeo
		"aguacate": "palta",         // sinónimo regional
		"avocado":  "palta",         // nombre en inglés
		"limon":    "jugo-limon",    // sin acento
		"LIMÓN":    "jugo-limon",
	}
	for q, want := range tests {
		got, err := foods.Search(ctx, "", q, 5)
		if err != nil {
			t.Fatalf("Search(%q): %v", q, err)
		}
		if len(got) == 0 || got[0].ID != want {
			t.Errorf("Search(%q) = %v, se esperaba primero %s", q, ids(got), want)
		}
	}
	if got, _ := foods.Search(ctx, "", "zzzz", 5); len(got) != 0 {
		t.Errorf("sin coincidencias = %v", ids(got))
	}

	f, err := foods.Get(ctx, "pechuga-pollo")
	if err != nil || *f.Micros.PotassiumMg != 256 || f.Micros.IronMg != nil || !f.Traits.ContainsMeat {
		t.Errorf("Get = %+v, %v (los micros sin dato deben volver como nil)", f, err)
	}
	if _, err := foods.Get(ctx, "no-existe"); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("inexistente = %v", err)
	}
}

func ids(fs []domain.Food) []domain.FoodID {
	out := make([]domain.FoodID, len(fs))
	for i, f := range fs {
		out[i] = f.ID
	}
	return out
}

func TestEntries(t *testing.T) {
	s, pool := testStore(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE foods CASCADE"); err != nil {
		t.Fatal(err)
	}
	seedFoods(t, s)
	alice, _ := s.CreateUser(ctx)
	bob, _ := s.CreateUser(ctx)
	pollo, _ := s.Foods().Get(ctx, "pechuga-pollo")
	entries := s.Entries()

	at := time.Date(2026, 11, 3, 15, 0, 0, 0, time.UTC)
	e, _ := domain.NewEntry(pollo, 150, at, domain.SourceManual)
	id, err := entries.Insert(ctx, alice, e)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	other, _ := domain.NewEntry(pollo, 100, at.Add(-24*time.Hour), domain.SourceManual)
	if _, err := entries.Insert(ctx, alice, other); err != nil {
		t.Fatal(err)
	}

	list, err := entries.List(ctx, alice, at.Add(-time.Hour), at.Add(time.Hour))
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %d comidas, %v", len(list), err)
	}
	got := list[0]
	if got.ID != id || got.Grams != 150 || got.Macros.Calories != 247.5 || got.FoodNameES != "Pechuga de pollo asada" ||
		*got.Micros.PotassiumMg != 384 || got.Micros.IronMg != nil || !got.EatenAt.Equal(at) {
		t.Errorf("comida leída = %+v", got)
	}
	if l, _ := entries.List(ctx, bob, at.Add(-time.Hour), at.Add(time.Hour)); len(l) != 0 {
		t.Error("Bob no ve las comidas de Alice")
	}
	if err := entries.Delete(ctx, bob, id); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("Bob borrando la comida de Alice = %v", err)
	}
	if err := entries.Delete(ctx, alice, id); err != nil {
		t.Errorf("Delete: %v", err)
	}
	if err := entries.Delete(ctx, alice, "no-es-uuid"); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("id inválido = %v", err)
	}
	bad := e
	bad.FoodID = "no-existe"
	if _, err := entries.Insert(ctx, alice, bad); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("alimento inexistente = %v", err)
	}
}

func TestSyncCatalogOnlyWhenChanged(t *testing.T) {
	s, pool := testStore(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE foods CASCADE; DELETE FROM app_meta"); err != nil {
		t.Fatal(err)
	}
	foods, err := catalog.Parse(data.FoodsSeed)
	if err != nil {
		t.Fatalf("el catálogo embebido no es válido: %v", err)
	}
	if imported, err := s.SyncCatalog(ctx, foods, "v1"); err != nil || !imported {
		t.Fatalf("primera carga: imported=%v err=%v", imported, err)
	}
	if imported, _ := s.SyncCatalog(ctx, foods, "v1"); imported {
		t.Error("con la misma versión no debe volver a importar")
	}
	if imported, _ := s.SyncCatalog(ctx, foods, "v2"); !imported {
		t.Error("una versión nueva se importa")
	}
	var n int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM foods").Scan(&n)
	if n != len(foods) || n < 100 {
		t.Errorf("alimentos = %d, catálogo = %d", n, len(foods))
	}
	// Con el catálogo real, incluidos los platos compuestos y los nombres que devuelve la IA.
	for q, want := range map[string]domain.FoodID{
		"aguacate":          "palta",
		"salsa chimichurri": "chimichurri",
		"chimichurri sauce": "chimichurri",
		"empanada de carne": "empanada-carne",
		"milanesa de pollo": "milanesa-pollo-frita",
		"ñoquis":            "noquis",
		"asado de tira":     "asado-tira",
		"beef steak":        "bife-solomillo", // no la milanesa ("breaded steak" + "beef milanesa")
		"steak":             "bife-solomillo", // el término exacto antes que "flank steak" (vacío)
		"bife":              "bife-solomillo",
		"mixed salad":       "ensalada-mixta",
		"pechga":            "pechuga-pollo", // error de tipeo
	} {
		got, err := s.Foods().Search(ctx, "", q, 3)
		if err != nil || len(got) == 0 || got[0].ID != want {
			t.Errorf("Search(%q) = %v %v, se esperaba primero %s", q, ids(got), err, want)
		}
	}
}

// Lo que devuelve la IA se cruza con sus dos nombres; gana la coincidencia más fuerte.
func TestMatchFoodWithRealCatalog(t *testing.T) {
	s, pool := testStore(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE foods CASCADE; DELETE FROM app_meta"); err != nil {
		t.Fatal(err)
	}
	foods, _ := catalog.Parse(data.FoodsSeed)
	if _, err := s.SyncCatalog(ctx, foods, "v1"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		names []string
		want  domain.FoodID // vacío = sin coincidencia
	}{
		{[]string{"Bife", "beef steak"}, "bife-solomillo"}, // "comí un bife con una ensalada mixta"
		{[]string{"Ensalada mixta", "mixed salad"}, "ensalada-mixta"},
		{[]string{"Milanesa", "breaded steak"}, "milanesa-carne-frita"},
		{[]string{"salsa chimichurri", "chimichurri sauce"}, "chimichurri"},
		{[]string{"", "white rice"}, "arroz-blanco"},
		{[]string{"Glaseado", "dragon glaze"}, ""},
		// Casos reales del 9 de noviembre de 2026: un estofado terminaba como "Bife magro
		// a la parrilla", "Puré de papas" y "Ketchup". Ahora no coinciden y se usa la
		// estimación de la IA.
		{[]string{"Carne del estofado", "stewed beef"}, ""},
		{[]string{"Papas guisadas", "stewed potatoes"}, ""},
		{[]string{"Pancake proteico", "protein pancake"}, ""},
		{[]string{"", ""}, ""},
	}
	for _, tt := range tests {
		got, ok, err := s.Foods().Match(ctx, "", tt.names)
		if err != nil {
			t.Fatal(err)
		}
		if (tt.want == "") == ok || (ok && got.ID != tt.want) {
			t.Errorf("Match(%q) = %s (%v), se esperaba %q", tt.names, got.ID, ok, tt.want)
		}
	}
}

func TestMatchAndEstimates(t *testing.T) {
	s, pool := testStore(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE foods CASCADE; TRUNCATE food_misses"); err != nil {
		t.Fatal(err)
	}
	seedFoods(t, s)
	foods := s.Foods()

	// Mismas palabras (sin "cocido", plurales ni acentos): coincide.
	if f, ok, err := foods.Match(ctx, "", []string{"Pechuga de pollo", "roasted chicken breast"}); err != nil || !ok || f.ID != "pechuga-pollo" {
		t.Errorf("Match pechuga = %v, %v, %v", f.ID, ok, err)
	}
	if f, ok, _ := foods.Match(ctx, "", []string{"jugo de limon"}); !ok || f.ID != "jugo-limon" {
		t.Errorf("Match limón = %v, %v", f.ID, ok)
	}
	// Solo compartir una palabra no alcanza: se usa la estimación de la IA.
	for _, q := range [][]string{{"Pollo al curry", "chicken curry"}, {"Jugo de naranja", "orange juice"}} {
		if f, ok, _ := foods.Match(ctx, "", q); ok {
			t.Errorf("Match(%v) = %v; no debería coincidir", q, f.ID)
		}
	}

	alice, _ := s.CreateUser(ctx)
	est, err := domain.NewEstimatedFood(alice, "Pancake proteico", "protein pancake",
		domain.NutrientEstimate{ProteinG: 20, CarbsG: 25, FiberG: 2, FatG: 5})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := foods.SaveEstimate(ctx, est)
	if err != nil {
		t.Fatalf("SaveEstimate: %v", err)
	}
	if saved.Owner != alice || !saved.Estimated() || saved.Per100g.Calories != 221 {
		t.Errorf("guardado = %+v", saved)
	}
	// Si ya existe, gana el anterior (el mismo alimento conserva sus valores).
	changed := est
	changed.Per100g.Calories = 999
	if again, _ := foods.SaveEstimate(ctx, changed); again.Per100g.Calories != 221 {
		t.Errorf("la segunda estimación pisó la primera: %+v", again.Per100g)
	}
	// Propio: su dueño lo encuentra (primero) y lo cruza; para otro usuario no existe.
	bob, _ := s.CreateUser(ctx)
	if found, _ := foods.Search(ctx, alice, "pancake", 10); len(found) != 1 || found[0].ID != saved.ID {
		t.Errorf("Alice buscando su pancake = %v", ids(found))
	}
	if found, _ := foods.Search(ctx, bob, "pancake", 10); len(found) != 0 {
		t.Errorf("Bob encontró el pancake de Alice: %v", ids(found))
	}
	if f, ok, _ := foods.Match(ctx, alice, []string{"pancakes proteicos"}); !ok || f.ID != saved.ID {
		t.Errorf("Match de Alice = %v, %v", f.ID, ok)
	}
	if _, ok, _ := foods.Match(ctx, bob, []string{"Pancake proteico"}); ok {
		t.Error("Match de Bob devolvió el alimento de Alice")
	}
	// Lo propio gana sobre el catálogo: si Alice tiene su "Palta", se usa la suya.
	myAvocado, _ := domain.NewEstimatedFood(alice, "Palta", "avocado", domain.NutrientEstimate{CarbsG: 9, FiberG: 7, FatG: 15, ProteinG: 2})
	if _, err := foods.SaveEstimate(ctx, myAvocado); err != nil {
		t.Fatal(err)
	}
	if f, _, _ := foods.Match(ctx, alice, []string{"Palta"}); f.ID != myAvocado.ID {
		t.Errorf("Match de Alice = %v; se esperaba su palta", f.ID)
	}
	if f, _, _ := foods.Match(ctx, bob, []string{"Palta"}); f.ID != "palta" {
		t.Errorf("Match de Bob = %v; se esperaba la del catálogo", f.ID)
	}

	// Receta propia: se guarda, se actualiza y conserva su porción.
	pollo, _ := foods.Get(ctx, "pechuga-pollo")
	recipe, err := domain.NewUserRecipe(alice, "Pollo con palta", 2, []domain.RecipeIngredient{{Food: pollo, Grams: 200}, {Food: saved, Grams: 100}})
	if err != nil {
		t.Fatal(err)
	}
	gotRecipe, err := foods.SaveRecipe(ctx, recipe)
	if err != nil || !gotRecipe.IsRecipe() || gotRecipe.ServingGrams != 150 || gotRecipe.Owner != alice {
		t.Fatalf("SaveRecipe = %+v, %v", gotRecipe, err)
	}
	updated, _ := domain.NewUserRecipe(alice, "Pollo con palta", 3, []domain.RecipeIngredient{{Food: pollo, Grams: 300}})
	if again, _ := foods.SaveRecipe(ctx, updated); again.ServingGrams != 100 || again.Per100g.Calories != pollo.Per100g.Calories {
		t.Errorf("la receta no se actualizó: %+v", again)
	}

	// Lo que no estaba en el catálogo se cuenta, sin usuario.
	for range 2 {
		if err := foods.RecordMiss(ctx, "Protein Pancake"); err != nil {
			t.Fatal(err)
		}
	}
	var misses int
	if err := pool.QueryRow(ctx, "SELECT misses FROM food_misses WHERE name = 'protein pancake'").Scan(&misses); err != nil || misses != 2 {
		t.Errorf("faltantes = %d, %v", misses, err)
	}

	// Anotarlo y borrar la cuenta: se van la comida y el alimento.
	e, _ := domain.NewEntry(saved, 120, time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC), domain.SourceVoice)
	if _, err := s.Entries().Insert(ctx, alice, e); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(ctx, alice); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := foods.Get(ctx, saved.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("el alimento estimado sobrevivió a la cuenta: %v", err)
	}
}
