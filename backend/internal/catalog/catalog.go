// Package catalog define el formato del catálogo de alimentos ya resuelto
// (data/foods_seed.json): lo escribe cmd/importfoods y lo lee la API al arrancar.
package catalog

import (
	"encoding/json"
	"fmt"

	"github.com/whiteowltech/owli/backend/internal/domain"
)

// Food es un alimento con sus nutrientes por 100 g. Los micros en null = sin dato.
type Food struct {
	ID        string   `json:"id"`
	NameES    string   `json:"nameEs"`
	NameEN    string   `json:"nameEn"`
	Aliases   []string `json:"aliases"`
	Source    string   `json:"source"`
	SourceRef string   `json:"sourceRef"`
	Per100g   struct {
		Kcal     float64 `json:"kcal"`
		ProteinG float64 `json:"proteinG"`
		CarbsG   float64 `json:"carbsG"`
		FiberG   float64 `json:"fiberG"`
		FatG     float64 `json:"fatG"`
	} `json:"per100g"`
	Micros struct {
		VitaminCMg   *float64 `json:"vitaminCMg"`
		CalciumMg    *float64 `json:"calciumMg"`
		IronMg       *float64 `json:"ironMg"`
		MagnesiumMg  *float64 `json:"magnesiumMg"`
		PotassiumMg  *float64 `json:"potassiumMg"`
		SodiumMg     *float64 `json:"sodiumMg"`
		ZincMg       *float64 `json:"zincMg"`
		VitaminB12Ug *float64 `json:"vitaminB12Ug"`
	} `json:"micros"`
	Traits domain.FoodTraits `json:"traits"`
}

// FromDomain arma la entrada del catálogo.
func FromDomain(f domain.Food, aliases []string, source, sourceRef string) Food {
	out := Food{ID: string(f.ID), NameES: f.NameES, NameEN: f.NameEN, Aliases: aliases, Source: source, SourceRef: sourceRef, Traits: f.Traits}
	out.Per100g.Kcal, out.Per100g.ProteinG, out.Per100g.CarbsG = f.Per100g.Calories, f.Per100g.ProteinG, f.Per100g.CarbsG
	out.Per100g.FiberG, out.Per100g.FatG = f.Per100g.FiberG, f.Per100g.FatG
	m := f.Micros
	out.Micros.VitaminCMg, out.Micros.CalciumMg, out.Micros.IronMg, out.Micros.MagnesiumMg = m.VitaminCMg, m.CalciumMg, m.IronMg, m.MagnesiumMg
	out.Micros.PotassiumMg, out.Micros.SodiumMg, out.Micros.ZincMg, out.Micros.VitaminB12Ug = m.PotassiumMg, m.SodiumMg, m.ZincMg, m.VitaminB12Ug
	return out
}

// Domain convierte la entrada al modelo de dominio y la valida.
func (c Food) Domain() (domain.Food, error) {
	f := domain.Food{
		ID: domain.FoodID(c.ID), NameES: c.NameES, NameEN: c.NameEN, Traits: c.Traits,
		Per100g: domain.Macros{Calories: c.Per100g.Kcal, ProteinG: c.Per100g.ProteinG, CarbsG: c.Per100g.CarbsG,
			FiberG: c.Per100g.FiberG, FatG: c.Per100g.FatG},
		Micros: domain.Micros{VitaminCMg: c.Micros.VitaminCMg, CalciumMg: c.Micros.CalciumMg, IronMg: c.Micros.IronMg,
			MagnesiumMg: c.Micros.MagnesiumMg, PotassiumMg: c.Micros.PotassiumMg, SodiumMg: c.Micros.SodiumMg,
			ZincMg: c.Micros.ZincMg, VitaminB12Ug: c.Micros.VitaminB12Ug},
	}
	if err := f.Per100g.Validate(); err != nil {
		return domain.Food{}, fmt.Errorf("alimento %s: %w", c.ID, err)
	}
	return f, nil
}

// Parse lee el catálogo y valida cada alimento.
func Parse(raw []byte) ([]Food, error) {
	var foods []Food
	if err := json.Unmarshal(raw, &foods); err != nil {
		return nil, fmt.Errorf("leyendo catálogo: %w", err)
	}
	seen := map[string]bool{}
	for _, f := range foods {
		if seen[f.ID] {
			return nil, fmt.Errorf("catálogo: id repetido %q", f.ID)
		}
		seen[f.ID] = true
		if _, err := f.Domain(); err != nil {
			return nil, err
		}
	}
	return foods, nil
}
