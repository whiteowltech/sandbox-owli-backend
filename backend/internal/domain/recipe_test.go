package domain

import (
	"errors"
	"math"
	"testing"
)

func fp(v float64) *float64 { return &v }

func TestComposeRecipe(t *testing.T) {
	oil := RecipePart{Per100g: Macros{Calories: 884, FatG: 100}, Micros: Micros{SodiumMg: fp(0), IronMg: fp(0.6)}}
	parsley := RecipePart{Per100g: Macros{Calories: 36, ProteinG: 3, CarbsG: 6.3, FiberG: 3.3, FatG: 0.8}, Micros: Micros{SodiumMg: fp(56)}}
	water := RecipePart{Per100g: Macros{}, Micros: Micros{SodiumMg: fp(4), IronMg: fp(0)}}

	with := func(p RecipePart, g float64) RecipePart { p.Grams = g; return p }

	tests := []struct {
		name     string
		parts    []RecipePart
		yield    float64
		wantKcal float64
		wantFat  float64
		wantNa   *float64 // sodio por 100 g; nil = sin dato
		wantFe   *float64
		wantErr  bool
	}{
		{
			name:     "sin rendimiento: se reparte en la suma de ingredientes",
			parts:    []RecipePart{with(oil, 50), with(water, 50)},
			wantKcal: 442, wantFat: 50, wantNa: fp(2), wantFe: fp(0.3),
		},
		{
			name:     "la cocción pierde agua: los nutrientes se concentran",
			parts:    []RecipePart{with(oil, 50), with(water, 50)},
			yield:    50,
			wantKcal: 884, wantFat: 100, wantNa: fp(4), wantFe: fp(0.6),
		},
		{
			name:     "un micro faltante en un ingrediente deja el plato sin dato, no en cero",
			parts:    []RecipePart{with(oil, 50), with(parsley, 50)},
			wantKcal: 460, wantFat: 50.4, wantNa: fp(28), wantFe: nil,
		},
		{name: "sin ingredientes", wantErr: true},
		{name: "gramos en cero", parts: []RecipePart{with(oil, 0)}, wantErr: true},
		{name: "rendimiento absurdo", parts: []RecipePart{with(oil, 100)}, yield: 10, wantErr: true},
		{name: "rendimiento mayor al doble", parts: []RecipePart{with(oil, 100)}, yield: 250, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, mi, err := ComposeRecipe(tt.parts, tt.yield)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidMacros) {
					t.Fatalf("esperaba ErrInvalidMacros, obtuve %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			near := func(what string, got, want float64) {
				if math.Abs(got-want) > 0.01 {
					t.Errorf("%s = %.2f, quería %.2f", what, got, want)
				}
			}
			near("kcal", m.Calories, tt.wantKcal)
			near("grasa", m.FatG, tt.wantFat)
			for _, c := range []struct {
				what      string
				got, want *float64
			}{{"sodio", mi.SodiumMg, tt.wantNa}, {"hierro", mi.IronMg, tt.wantFe}} {
				switch {
				case c.want == nil && c.got != nil:
					t.Errorf("%s = %.2f, quería sin dato", c.what, *c.got)
				case c.want != nil && c.got == nil:
					t.Errorf("%s sin dato, quería %.2f", c.what, *c.want)
				case c.want != nil:
					near(c.what, *c.got, *c.want)
				}
			}
			if mi.VitaminB12Ug != nil {
				t.Error("ningún ingrediente tiene B12: debe quedar sin dato")
			}
		})
	}
}
