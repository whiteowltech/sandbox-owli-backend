package domain

import "fmt"

// RecipePart es un ingrediente de una receta: sus valores por 100 g y los gramos que lleva.
type RecipePart struct {
	Per100g Macros
	Micros  Micros
	Grams   float64
}

// Límites del rendimiento (peso final del plato) respecto de la suma de ingredientes:
// la cocción pierde agua (horno, plancha) o la absorbe (hervido), pero nunca en esta
// proporción. Fuera de este rango es casi seguro un error de carga.
const (
	minRecipeYield = 0.3
	maxRecipeYield = 2.0
)

// ComposeRecipe calcula los valores por 100 g de un plato a partir de sus ingredientes.
// Sirve para los platos que USDA no tiene (chimichurri, empanadas, milanesas…).
//
// Reglas:
//   - yieldG es el peso del plato terminado; 0 = la suma de los ingredientes. Se asume
//     que la cocción cambia el agua, no los nutrientes: el total se reparte en yieldG.
//   - Un micro solo se conoce si TODOS los ingredientes lo tienen: sumar un faltante
//     como cero subestimaría el plato ("sin dato" nunca es cero).
func ComposeRecipe(parts []RecipePart, yieldG float64) (Macros, Micros, error) {
	if len(parts) == 0 {
		return Macros{}, Micros{}, fmt.Errorf("%w: receta sin ingredientes", ErrInvalidMacros)
	}
	var total Macros
	var micros MicroTotals
	sumG := 0.0
	for i, p := range parts {
		if p.Grams <= 0 {
			return Macros{}, Micros{}, fmt.Errorf("%w: ingrediente %d con %.1f g", ErrInvalidMacros, i+1, p.Grams)
		}
		if err := p.Per100g.Validate(); err != nil {
			return Macros{}, Micros{}, fmt.Errorf("ingrediente %d: %w", i+1, err)
		}
		k := p.Grams / 100
		total = total.Add(p.Per100g.Scale(k))
		micros.Add(p.Micros.Scale(k))
		sumG += p.Grams
	}
	if yieldG == 0 {
		yieldG = sumG
	}
	if yieldG < sumG*minRecipeYield || yieldG > sumG*maxRecipeYield {
		return Macros{}, Micros{}, fmt.Errorf("%w: rendimiento de %.0f g para %.0f g de ingredientes", ErrInvalidMacros, yieldG, sumG)
	}

	per100 := 100 / yieldG
	out := micros.Sum.Scale(per100)
	sum := out.fields()
	for i := range sum {
		if micros.Known[i] < micros.Items {
			*sum[i] = nil
		}
	}
	m := total.Scale(per100)
	return m, out, m.Validate()
}
