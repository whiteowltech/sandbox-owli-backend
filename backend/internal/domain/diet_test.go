package domain

import (
	"errors"
	"math"
	"testing"
)

func TestDietPermits(t *testing.T) {
	var (
		steak   = FoodTraits{AnimalOrigin: true, ContainsMeat: true}
		salmon  = FoodTraits{AnimalOrigin: true, ContainsFish: true}
		eggs    = FoodTraits{AnimalOrigin: true}
		rice    = FoodTraits{PlantOrigin: true, ContainsGrain: true}
		burger  = FoodTraits{AnimalOrigin: true, PlantOrigin: true, ContainsMeat: true} // carne + pan
		cheeseP = FoodTraits{AnimalOrigin: true, PlantOrigin: true}                     // pizza margarita
		salt    = FoodTraits{}                                                          // mineral: ni animal ni vegetal
		coffee  = FoodTraits{PlantOrigin: true}
		liverC  = FoodTraits{AnimalOrigin: true, ContainsMeat: true} // cápsulas de hígado desecado
	)
	tests := []struct {
		diet DietType
		food FoodTraits
		name string
		want bool
	}{
		{DietCarnivore, steak, "carnívoro/bife", true},
		{DietCarnivore, eggs, "carnívoro/huevos", true},
		{DietCarnivore, rice, "carnívoro/arroz", false},
		{DietCarnivore, burger, "carnívoro/hamburguesa con pan", false},
		{DietCarnivore, salt, "carnívoro/sal, agua o magnesio", true},
		{DietCarnivore, liverC, "carnívoro/cápsulas de hígado", true},
		{DietCarnivore, coffee, "carnívoro/café", false},
		{DietVegan, salt, "vegano/sal", true},
		{DietVegan, rice, "vegano/arroz", true},
		{DietVegan, eggs, "vegano/huevos", false},
		{DietVegetarian, cheeseP, "vegetariano/pizza", true},
		{DietVegetarian, salmon, "vegetariano/salmón", false},
		{DietVegetarian, steak, "vegetariano/bife", false},
		{DietKeto, rice, "keto/arroz (se controla por carbohidratos netos)", true},
		{DietOmnivore, burger, "omnívoro/hamburguesa", true},
		{DietPescetarian, salmon, "pescetariano/salmón", true},
		{DietPescetarian, eggs, "pescetariano/huevos", true},
		{DietPescetarian, steak, "pescetariano/bife", false},
		{DietPaleo, steak, "paleo/bife", true},
		{DietPaleo, rice, "paleo/arroz", false},
		{DietPaleo, FoodTraits{PlantOrigin: true, ContainsLegume: true}, "paleo/lentejas", false},
		{DietPaleo, FoodTraits{AnimalOrigin: true, ContainsDairy: true}, "paleo/queso", false},
		{DietPaleo, FoodTraits{PlantOrigin: true, ContainsAddedSugar: true}, "paleo/galletitas", false},
		{DietPaleo, FoodTraits{PlantOrigin: true}, "paleo/batata", true},
		{DietType("frugivoro"), steak, "dieta desconocida", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.diet.Permits(tt.food); got != tt.want {
				t.Errorf("Permits = %v, se esperaba %v", got, tt.want)
			}
		})
	}
}

func TestParseDiet(t *testing.T) {
	if d, err := ParseDiet("keto"); err != nil || d != DietKeto {
		t.Errorf("ParseDiet(keto) = %q, %v", d, err)
	}
	if d, err := ParseDiet("paleo"); err != nil || d != DietPaleo {
		t.Errorf("ParseDiet(paleo) = %q, %v", d, err)
	}
	if _, err := ParseDiet("frugivoro"); !errors.Is(err, ErrInvalidDiet) {
		t.Errorf("ParseDiet(frugivoro) = %v, se esperaba ErrInvalidDiet", err)
	}
}

func TestRestrictionPermits(t *testing.T) {
	var (
		bread      = FoodTraits{PlantOrigin: true, ContainsGrain: true, ContainsGluten: true, AllergensVerified: true}
		rice       = FoodTraits{PlantOrigin: true, ContainsGrain: true, AllergensVerified: true}
		milk       = FoodTraits{AnimalOrigin: true, ContainsDairy: true, ContainsLactose: true, AllergensVerified: true}
		agedCheese = FoodTraits{AnimalOrigin: true, ContainsDairy: true, AllergensVerified: true} // curado: lactosa despreciable
		unknown    = FoodTraits{PlantOrigin: true}                                                // sin verificar
	)
	tests := []struct {
		name string
		r    Restriction
		food FoodTraits
		want bool
	}{
		{"sin gluten/pan", RestrictionGlutenFree, bread, false},
		{"sin gluten/arroz", RestrictionGlutenFree, rice, true},
		{"sin gluten/alimento sin verificar", RestrictionGlutenFree, unknown, false},
		{"sin lactosa/leche", RestrictionLactoseFree, milk, false},
		{"sin lactosa/queso curado", RestrictionLactoseFree, agedCheese, true},
		{"sin lactosa/alimento sin verificar", RestrictionLactoseFree, unknown, false},
		{"restricción desconocida", Restriction("sin_todo"), rice, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.Permits(tt.food); got != tt.want {
				t.Errorf("Permits = %v, se esperaba %v", got, tt.want)
			}
		})
	}
}

func TestAllows(t *testing.T) {
	glutenFree := []Restriction{RestrictionGlutenFree}
	seitan := FoodTraits{PlantOrigin: true, ContainsGrain: true, ContainsGluten: true, AllergensVerified: true}
	tofu := FoodTraits{PlantOrigin: true, ContainsLegume: true, AllergensVerified: true}
	steak := FoodTraits{AnimalOrigin: true, ContainsMeat: true, AllergensVerified: true}
	if Allows(DietVegan, glutenFree, seitan) {
		t.Error("vegano sin gluten no debería permitir seitán")
	}
	if !Allows(DietVegan, glutenFree, tofu) {
		t.Error("vegano sin gluten debería permitir tofu")
	}
	if Allows(DietVegan, glutenFree, steak) {
		t.Error("la dieta se aplica aunque la restricción lo permita")
	}
	if !Allows(DietOmnivore, nil, seitan) {
		t.Error("sin restricciones, omnívoro permite seitán")
	}
	if _, err := ParseRestriction("gluten_free"); err != nil {
		t.Errorf("ParseRestriction(gluten_free) = %v", err)
	}
	if _, err := ParseRestriction("sin_todo"); !errors.Is(err, ErrInvalidRestriction) {
		t.Errorf("ParseRestriction(sin_todo) = %v, se esperaba ErrInvalidRestriction", err)
	}
}

func TestKetoFits(t *testing.T) {
	day := Macros{CarbsG: 22, FiberG: 4} // 18 g netos
	tests := []struct {
		name    string
		portion Macros
		want    bool
	}{
		{"queda por debajo", Macros{CarbsG: 8, FiberG: 2}, true}, // 24 g
		{"llega justo al límite", Macros{CarbsG: 7}, false},      // 25 g: el límite es estricto
		{"lo supera", Macros{CarbsG: 30, FiberG: 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := KetoFits(day, tt.portion); got != tt.want {
				t.Errorf("KetoFits = %v, se esperaba %v", got, tt.want)
			}
		})
	}
}

func TestForGrams(t *testing.T) {
	chicken := Macros{Calories: 165, ProteinG: 31, FatG: 3.6} // por 100 g
	got, err := ForGrams(chicken, 200)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if math.Abs(got.Calories-330) > 1e-9 || math.Abs(got.ProteinG-62) > 1e-9 {
		t.Errorf("ForGrams(200) = %+v", got)
	}
	if _, err := ForGrams(chicken, 0); !errors.Is(err, ErrInvalidMacros) {
		t.Errorf("ForGrams(0) = %v, se esperaba ErrInvalidMacros", err)
	}
	if _, err := ForGrams(Macros{CarbsG: 1, FiberG: 2}, 100); !errors.Is(err, ErrInvalidMacros) {
		t.Errorf("fibra > carbohidratos debería fallar, got %v", err)
	}
}
