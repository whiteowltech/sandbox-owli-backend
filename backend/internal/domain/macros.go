package domain

import "fmt"

// Macros es el aporte nutricional de una porción, una comida o un día.
// Los carbohidratos son totales; los netos se derivan restando la fibra.
type Macros struct {
	Calories float64
	ProteinG float64
	CarbsG   float64
	FiberG   float64
	FatG     float64
}

// Validate rechaza valores negativos o fibra mayor que los carbohidratos.
func (m Macros) Validate() error {
	if m.Calories < 0 || m.ProteinG < 0 || m.CarbsG < 0 || m.FiberG < 0 || m.FatG < 0 {
		return fmt.Errorf("%w: valores negativos", ErrInvalidMacros)
	}
	if m.FiberG > m.CarbsG {
		return fmt.Errorf("%w: fibra (%.1f g) mayor que carbohidratos (%.1f g)", ErrInvalidMacros, m.FiberG, m.CarbsG)
	}
	return nil
}

// NetCarbsG son los carbohidratos netos (totales − fibra), la métrica de Keto.
func (m Macros) NetCarbsG() float64 {
	return m.CarbsG - m.FiberG
}

// Add suma dos aportes.
func (m Macros) Add(o Macros) Macros {
	return Macros{
		Calories: m.Calories + o.Calories,
		ProteinG: m.ProteinG + o.ProteinG,
		CarbsG:   m.CarbsG + o.CarbsG,
		FiberG:   m.FiberG + o.FiberG,
		FatG:     m.FatG + o.FatG,
	}
}

// Scale multiplica todos los valores por factor. Se usa para pasar de los
// valores por 100 g de la base nutricional a los gramos reales de la porción.
func (m Macros) Scale(factor float64) Macros {
	return Macros{
		Calories: m.Calories * factor,
		ProteinG: m.ProteinG * factor,
		CarbsG:   m.CarbsG * factor,
		FiberG:   m.FiberG * factor,
		FatG:     m.FatG * factor,
	}
}

// ForGrams calcula los macros de una porción a partir de los valores por 100 g.
// Es la única vía por la que se obtienen macros: la IA nunca los calcula.
func ForGrams(per100g Macros, grams float64) (Macros, error) {
	if grams <= 0 {
		return Macros{}, fmt.Errorf("%w: gramos debe ser positivo (%.1f)", ErrInvalidMacros, grams)
	}
	if err := per100g.Validate(); err != nil {
		return Macros{}, err
	}
	return per100g.Scale(grams / 100), nil
}
