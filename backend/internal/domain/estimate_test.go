package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestNutrientEstimatePer100g(t *testing.T) {
	cases := []struct {
		name    string
		in      NutrientEstimate
		kcal    float64
		wantErr bool
	}{
		// Estofado de carne: 4·12 + 4·(6−1) + 2·1 + 9·6 = 124 kcal.
		{name: "estofado", in: NutrientEstimate{ProteinG: 12, CarbsG: 6, FiberG: 1, FatG: 6}, kcal: 124},
		// Pancake proteico: 4·20 + 4·(25−2) + 2·2 + 9·5 = 221 kcal.
		{name: "pancake proteico", in: NutrientEstimate{ProteinG: 20, CarbsG: 25, FiberG: 2, FatG: 5}, kcal: 221},
		// Cerveza: el alcohol suma 7 kcal/g (4·0,5 + 4·3,6 + 7·3,9 = 43,7).
		{name: "cerveza", in: NutrientEstimate{ProteinG: 0.5, CarbsG: 3.6, AlcoholG: 3.9}, kcal: 43.7},
		{name: "agua", in: NutrientEstimate{}, kcal: 0},
		// Aceite: 100 g de grasa, justo en el límite.
		{name: "aceite", in: NutrientEstimate{FatG: 100}, kcal: 900},
		{name: "redondeo de la IA dentro del margen", in: NutrientEstimate{ProteinG: 30, CarbsG: 40, FatG: 32}, kcal: 568},
		{name: "suma más de 100 g", in: NutrientEstimate{ProteinG: 60, CarbsG: 30, FatG: 20}, wantErr: true},
		{name: "más fibra que carbos", in: NutrientEstimate{CarbsG: 5, FiberG: 8}, wantErr: true},
		{name: "negativo", in: NutrientEstimate{ProteinG: -1}, wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := c.in.Per100g()
			if c.wantErr {
				if !errors.Is(err, ErrInvalidMacros) {
					t.Fatalf("err = %v, se esperaba ErrInvalidMacros", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err inesperado: %v", err)
			}
			if math.Abs(m.Calories-c.kcal) > 0.01 {
				t.Errorf("kcal = %.2f, se esperaba %.2f", m.Calories, c.kcal)
			}
			if m.ProteinG != c.in.ProteinG || m.CarbsG != c.in.CarbsG || m.FiberG != c.in.FiberG || m.FatG != c.in.FatG {
				t.Errorf("macros = %+v, no coinciden con la estimación %+v", m, c.in)
			}
		})
	}
}

func TestNewEstimatedFood(t *testing.T) {
	est := NutrientEstimate{ProteinG: 20, CarbsG: 25, FiberG: 2, FatG: 5}

	f, err := NewEstimatedFood("u-1", "pancake  proteico", "protein pancake", est)
	if err != nil {
		t.Fatal(err)
	}
	if f.NameES != "Pancake proteico" || f.NameEN != "Protein pancake" {
		t.Errorf("nombres = %q / %q", f.NameES, f.NameEN)
	}
	if !f.Estimated() || f.Owner != "u-1" || f.Traits.AllergensVerified {
		t.Errorf("debe ser privado, estimado y sin alérgenos verificados: %+v", f)
	}
	if !strings.HasPrefix(string(f.ID), "ai-") {
		t.Errorf("ID = %q", f.ID)
	}

	// El mismo nombre (con otros acentos y mayúsculas) es el mismo alimento.
	again, _ := NewEstimatedFood("u-1", "Pancake Protéico", "", est)
	if again.ID != f.ID {
		t.Errorf("el mismo alimento cambió de ID: %q y %q", f.ID, again.ID)
	}
	// Otro usuario tiene su propio alimento.
	other, _ := NewEstimatedFood("u-2", "pancake proteico", "", est)
	if other.ID == f.ID {
		t.Error("dos usuarios comparten un alimento estimado")
	}
	if !f.VisibleTo("u-1") || f.VisibleTo("u-2") {
		t.Error("un alimento estimado solo lo ve su dueño")
	}
	if !(Food{ID: "arroz"}).VisibleTo("u-2") {
		t.Error("el catálogo lo ve cualquiera")
	}

	if _, err := NewEstimatedFood("", "pancake", "", est); err == nil {
		t.Error("sin dueño debería fallar")
	}
	if _, err := NewEstimatedFood("u-1", " ¡! ", "", est); err == nil {
		t.Error("sin nombre debería fallar")
	}
	if _, err := NewEstimatedFood("u-1", "piedra", "", NutrientEstimate{ProteinG: 90, FatG: 90}); err == nil {
		t.Error("una estimación imposible debería fallar")
	}
}

func TestNamesMatch(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"white rice", "Cooked white rice", true},
		{"Arroz blanco", "Arroz blanco cocido", true},
		{"chicken breast", "chicken breast", true},
		{"boiled potatoes", "boiled potato", true},
		{"Papas", "papa", true},
		{"Pechuga de pollo", "pechuga pollo", true},
		{"Limón", "limon", true},
		// Lo que llevaba a errores: compartir una palabra no es ser el mismo alimento.
		{"stewed beef", "Lean beef steak", false},
		{"beef", "Bife magro a la parrilla", false},
		{"mashed potatoes", "stewed potatoes", false},
		{"tomato sauce", "tomato ketchup", false},
		{"pancake proteico", "panqueque", false},
		{"raw salmon", "salmon", false},
		{"", "", false},
		{"de la", "de la", false},
	}
	for _, c := range cases {
		if got := NamesMatch(c.a, c.b); got != c.want {
			t.Errorf("NamesMatch(%q, %q) = %v, se esperaba %v", c.a, c.b, got, c.want)
		}
	}
}
