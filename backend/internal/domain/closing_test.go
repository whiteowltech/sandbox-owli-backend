package domain

import (
	"fmt"
	"math"
	"testing"
	"time"
)

var (
	animalMeat = FoodTraits{AnimalOrigin: true, ContainsMeat: true, AllergensVerified: true}
	plain      = FoodTraits{PlantOrigin: true, AllergensVerified: true}

	cChicken = Food{ID: "pollo", Per100g: Macros{Calories: 165, ProteinG: 31, FatG: 3.6}, Traits: animalMeat}
	cSteak   = Food{ID: "bife", Per100g: Macros{Calories: 200, ProteinG: 30, FatG: 9}, Traits: animalMeat}
	cTofu    = Food{ID: "tofu", Per100g: Macros{Calories: 144, ProteinG: 17, CarbsG: 3, FiberG: 2, FatG: 9}, Traits: FoodTraits{PlantOrigin: true, ContainsLegume: true, AllergensVerified: true}}
	cRice    = Food{ID: "arroz", Per100g: Macros{Calories: 130, ProteinG: 2.7, CarbsG: 28.2, FiberG: 0.4, FatG: 0.3}, Traits: FoodTraits{PlantOrigin: true, ContainsGrain: true, AllergensVerified: true}}
	cBread   = Food{ID: "pan", Per100g: Macros{Calories: 266, ProteinG: 8.9, CarbsG: 49, FiberG: 2.7, FatG: 3.3}, Traits: FoodTraits{PlantOrigin: true, ContainsGrain: true, ContainsGluten: true}}
	cAvocado = Food{ID: "palta", Per100g: Macros{Calories: 160, ProteinG: 2, CarbsG: 8.5, FiberG: 6.7, FatG: 14.7}, Traits: plain}
	cBrocoli = Food{ID: "brocoli", Per100g: Macros{Calories: 35, ProteinG: 2.4, CarbsG: 7.2, FiberG: 3.3, FatG: 0.4}, Traits: plain}
	cOlive   = Food{ID: "oliva", Per100g: Macros{Calories: 884, FatG: 100}, Traits: plain}
	cButter  = Food{ID: "manteca", Per100g: Macros{Calories: 717, ProteinG: 0.9, CarbsG: 0.1, FatG: 81}, Traits: FoodTraits{AnimalOrigin: true, ContainsDairy: true, AllergensVerified: true}}
)

func dish(id string, breakfast bool, foods ...Food) DishTemplate {
	t := DishTemplate{ID: id, NameES: id, Breakfast: breakfast}
	for i, f := range foods {
		c := DishComponent{Food: f, MinG: 30, MaxG: 350}
		if f.ID == "oliva" || f.ID == "manteca" {
			c.MinG, c.MaxG = 3, 30
		}
		if i == 0 {
			c.MinG = 50
		}
		t.Components = append(t.Components, c)
	}
	return t
}

var testDishes = []DishTemplate{
	dish("pollo-arroz-palta", false, cChicken, cRice, cAvocado),
	dish("pollo-arroz-oliva", false, cChicken, cRice, cOlive),
	dish("pollo-brocoli-palta", false, cChicken, cBrocoli, cAvocado),
	dish("bife-brocoli-manteca", false, cSteak, cBrocoli, cButter),
	dish("bife-manteca", false, cSteak, cButter),
	dish("tofu-arroz-palta", false, cTofu, cRice, cAvocado),
	dish("tostadas-palta-tofu", true, cTofu, cBread, cAvocado),
}

func TestMealsLeft(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 11, 4, h, m, 0, 0, time.UTC) }
	tests := []struct {
		name    string
		perDay  int
		entries []time.Time
		want    int
	}{
		{"nada anotado", 4, nil, 4},
		{"dos anotaciones juntas son una comida", 4, []time.Time{at(8, 0), at(8, 40)}, 3},
		{"desordenadas", 4, []time.Time{at(13, 0), at(8, 0), at(8, 30)}, 2},
		{"más comidas que las configuradas: queda la próxima", 3, []time.Time{at(8, 0), at(12, 0), at(16, 0), at(20, 0)}, 1},
		{"OMAD", 1, nil, 1},
	}
	for _, tt := range tests {
		if got := MealsLeft(tt.perDay, tt.entries); got != tt.want {
			t.Errorf("%s: MealsLeft = %d, quería %d", tt.name, got, tt.want)
		}
	}
}

func TestMealNeed(t *testing.T) {
	targets := MacroTargets{ProteinG: 150, CarbsG: 220, FatG: 70}
	got := MealNeed(2100, targets, Macros{Calories: 1100, ProteinG: 90, CarbsG: 100, FatG: 80}, 2)
	want := Macros{Calories: 500, ProteinG: 30, CarbsG: 60, FatG: 0} // la grasa ya se pasó: no es negativa
	if got != want {
		t.Errorf("MealNeed = %+v, quería %+v", got, want)
	}
	keto := MealNeed(1800, MacroTargets{ProteinG: 120, CarbsG: 20, FatG: 140, NetCarbs: true}, Macros{CarbsG: 15, FiberG: 7}, 1)
	if keto.CarbsG != 12 {
		t.Errorf("en keto cuentan los carbos netos: %v", keto.CarbsG)
	}
}

func TestSuggestDishesClosesTheGap(t *testing.T) {
	need := Macros{Calories: 600, ProteinG: 45, CarbsG: 55, FatG: 18}
	got := SuggestDishes(testDishes, ClosingRequest{Need: need, Diet: DietOmnivore})
	if len(got) == 0 {
		t.Fatal("sin sugerencias")
	}
	best := got[0]
	if !best.Exact {
		t.Errorf("el mejor plato debería cerrar dentro de la tolerancia: %s %+v", best.ID, best.Macros)
	}
	for _, s := range got {
		inTol := math.Abs(s.Macros.Calories-need.Calories) <= need.Calories*ClosingCalorieTolerance &&
			math.Abs(s.Macros.ProteinG-need.ProteinG) <= ClosingProteinToleranceG
		if s.Exact != inTol {
			t.Errorf("%s: Exact = %v pero está %s de tolerancia (%+v)", s.ID, s.Exact, map[bool]string{true: "dentro", false: "fuera"}[inTol], s.Macros)
		}
		for i, p := range s.Portions {
			if math.Mod(p.Grams, 5) != 0 || p.Grams < 3 {
				t.Errorf("%s: porción %d de %.1f g (se redondea a 5 g)", s.ID, i, p.Grams)
			}
		}
	}
	// Variedad: como mucho 2 platos con la misma fuente de proteína.
	count := map[FoodID]int{}
	for _, s := range got {
		count[s.Portions[0].Food.ID]++
	}
	for id, n := range count {
		if n > 2 {
			t.Errorf("%d platos con %s", n, id)
		}
	}
}

func TestSuggestDishesRespectsDietAndRestrictions(t *testing.T) {
	need := Macros{Calories: 600, ProteinG: 45, CarbsG: 40, FatG: 25}
	tests := []struct {
		diet         DietType
		restrictions []Restriction
		forbidden    []FoodID
	}{
		{DietVegan, nil, []FoodID{"pollo", "bife", "manteca"}},
		{DietCarnivore, nil, []FoodID{"arroz", "palta", "brocoli", "tofu", "pan", "oliva"}},
		{DietPaleo, nil, []FoodID{"arroz", "pan", "tofu", "manteca"}},
		{DietOmnivore, []Restriction{RestrictionGlutenFree}, []FoodID{"pan"}},
		{DietKeto, nil, []FoodID{"arroz", "pan"}}, // demasiados carbos netos por 100 g
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.diet, tt.restrictions), func(t *testing.T) {
			got := SuggestDishes(testDishes, ClosingRequest{Need: need, Diet: tt.diet, Restrictions: tt.restrictions})
			for _, s := range got {
				for _, p := range s.Portions {
					if !Allows(tt.diet, tt.restrictions, p.Food.Traits) {
						t.Errorf("%s incluye %s, que la dieta no permite", s.ID, p.Food.ID)
					}
					for _, f := range tt.forbidden {
						if p.Food.ID == f {
							t.Errorf("%s incluye %s", s.ID, f)
						}
					}
				}
			}
		})
	}
	// Carnívoro: solo el bife con manteca, resuelto por proteína y calorías.
	got := SuggestDishes(testDishes, ClosingRequest{Need: Macros{Calories: 700, ProteinG: 50, FatG: 50}, Diet: DietCarnivore})
	if len(got) != 1 || got[0].ID != "bife-manteca" {
		t.Fatalf("carnívoro = %v", ids(got))
	}
}

func TestSuggestDishesEdgeCases(t *testing.T) {
	if got := SuggestDishes(testDishes, ClosingRequest{Need: Macros{Calories: 80, ProteinG: 5}, Diet: DietOmnivore}); got != nil {
		t.Errorf("día cerrado: no se sugiere nada, %v", ids(got))
	}
	// Casi sin proteína pendiente: la fuente de proteína pediría menos que su porción mínima.
	got := SuggestDishes(testDishes, ClosingRequest{Need: Macros{Calories: 300, ProteinG: 2, CarbsG: 60, FatG: 5}, Diet: DietOmnivore})
	for _, s := range got {
		if s.Portions[0].Grams < 50 {
			t.Errorf("%s con %v g de proteína", s.ID, s.Portions[0].Grams)
		}
	}
	// Al desayuno suben los platos de desayuno.
	need := Macros{Calories: 500, ProteinG: 25, CarbsG: 50, FatG: 20}
	breakfast := SuggestDishes(testDishes, ClosingRequest{Need: need, Diet: DietVegan, Breakfast: true})
	dinner := SuggestDishes(testDishes, ClosingRequest{Need: need, Diet: DietVegan})
	if pos(breakfast, "tostadas-palta-tofu") > pos(dinner, "tostadas-palta-tofu") {
		t.Errorf("al desayuno las tostadas no pueden quedar más abajo: %v vs %v", ids(breakfast), ids(dinner))
	}
}

func TestSuggestDishesCapsAtTen(t *testing.T) {
	var many []DishTemplate
	for i := range 30 {
		f := cChicken
		f.ID = FoodID(fmt.Sprintf("pollo-%d", i)) // fuentes distintas: no las corta la variedad
		many = append(many, dish(fmt.Sprintf("d%d", i), false, f, cRice, cAvocado))
	}
	if got := SuggestDishes(many, ClosingRequest{Need: Macros{Calories: 600, ProteinG: 45, CarbsG: 55, FatG: 18}, Diet: DietOmnivore}); len(got) != MaxSuggestions {
		t.Errorf("se sugieren como mucho %d: %d", MaxSuggestions, len(got))
	}
}

func ids(s []DishSuggestion) []string {
	var out []string
	for _, d := range s {
		out = append(out, d.ID)
	}
	return out
}

func pos(s []DishSuggestion, id string) int {
	for i, d := range s {
		if d.ID == id {
			return i
		}
	}
	return len(s)
}

func TestPlanNextMeal(t *testing.T) {
	ba, _ := time.LoadLocation("America/Argentina/Buenos_Aires")
	at := func(h, m int) time.Time { return time.Date(2026, 11, 4, h, m, 0, 0, ba) }
	tests := []struct {
		name      string
		perDay    int
		eaten     []time.Time
		now       time.Time
		left      int
		breakfast bool
	}{
		{"primera comida de la mañana: desayuno", 4, nil, at(8, 0), 4, true},
		{"a media mañana con el desayuno hecho", 4, []time.Time{at(8, 0)}, at(10, 30), 3, false},
		{"almuerzo tardío sin desayuno: no es desayuno", 3, nil, at(13, 0), 3, false},
		{"merienda: dos comidas hechas y la cena por delante", 4, []time.Time{at(8, 0), at(13, 0)}, at(17, 0), 2, true},
		{"con 3 al día, a las 17 h va la cena", 3, []time.Time{at(8, 0), at(13, 0)}, at(17, 0), 1, false},
		{"a las 20 h no entran tres comidas más", 4, []time.Time{at(13, 0)}, at(20, 0), 1, false},
		{"a las 19 h entran dos", 5, []time.Time{at(8, 0)}, at(19, 0), 2, false},
		{"pasadas las 22 h queda una", 4, nil, at(23, 0), 1, false},
	}
	for _, tt := range tests {
		got := PlanNextMeal(tt.perDay, tt.eaten, tt.now)
		if got.Left != tt.left || got.Breakfast != tt.breakfast {
			t.Errorf("%s: %+v, quería Left=%d Breakfast=%v", tt.name, got, tt.left, tt.breakfast)
		}
	}
}

func TestDayOver(t *testing.T) {
	targets := MacroTargets{ProteinG: 150, CarbsG: 192, FatG: 53}
	got := DayOver(2000, targets, Macros{Calories: 1500, ProteinG: 160, CarbsG: 2, FatG: 97})
	if want := (Macros{ProteinG: 10, FatG: 44}); got != want {
		t.Errorf("DayOver = %+v, quería %+v", got, want)
	}
}

// Caso real del 4 de noviembre de 2026: carbos 2/192 g y grasa 97/53 g. Sugería
// huevos con manteca; con la grasa pasada no puede haber nada que sume grasa.
func TestSuggestDishesWithFatAlreadyOver(t *testing.T) {
	need := Macros{Calories: 500, ProteinG: 8, CarbsG: 95}
	sides := append([]DishTemplate{
		{ID: "arroz-solo", Side: true, Components: []DishComponent{{Food: cRice, MinG: 100, MaxG: 350}}},
		{ID: "palta-sola", Side: true, Components: []DishComponent{{Food: cAvocado, MinG: 50, MaxG: 200}}},
	}, testDishes...)
	got := SuggestDishes(sides, ClosingRequest{Need: need, Over: Macros{FatG: 44}, Diet: DietOmnivore})
	if len(got) == 0 || got[0].ID != "arroz-solo" {
		t.Fatalf("con poca proteína pendiente y la grasa pasada va el arroz: %v", ids(got))
	}
	for _, s := range got {
		if s.Macros.FatG > OverMacroAllowanceG {
			t.Errorf("%s suma %.0f g de grasa con la grasa del día pasada", s.ID, s.Macros.FatG)
		}
	}
	if !got[0].Side || got[0].Portions[0].Grams < 300 {
		t.Errorf("el arroz se escala hasta las calorías (con tope): %+v", got[0].Portions)
	}
}

func TestSidesOnlyWhenLittleProteinIsMissing(t *testing.T) {
	side := DishTemplate{ID: "arroz-solo", Side: true, Components: []DishComponent{{Food: cRice, MinG: 100, MaxG: 350}}}
	if got := SuggestDishes([]DishTemplate{side}, ClosingRequest{Need: Macros{Calories: 600, ProteinG: 40, CarbsG: 60}, Diet: DietOmnivore}); len(got) != 0 {
		t.Errorf("faltan 40 g de proteína: un arroz solo no sirve, %v", ids(got))
	}
	got := SuggestDishes([]DishTemplate{side}, ClosingRequest{Need: Macros{Calories: 260, ProteinG: 5, CarbsG: 55}, Diet: DietOmnivore})
	if len(got) != 1 || got[0].Portions[0].Grams != 200 {
		t.Errorf("260 kcal de arroz = 200 g: %+v", got)
	}
}
