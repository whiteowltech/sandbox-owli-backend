package domain

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// ErrInvalidRecognition indica que la respuesta de la IA no cumple el contrato.
var ErrInvalidRecognition = errors.New("reconocimiento de alimentos inválido")

// LowConfidenceThreshold: por debajo de este valor un ítem se resalta en la
// pantalla de confirmación. Solo es una ayuda visual: el benchmark (noviembre
// de 2026) mostró que la confianza del modelo no se correlaciona con el error
// real, así que nunca decide reintentos ni permite saltarse la confirmación.
const LowConfidenceThreshold = 0.6

// RecognizedFood es un alimento identificado por la IA: qué es, cuántos gramos y
// una estimación de su composición cada 100 g. La estimación solo se usa cuando el
// alimento no está en la base nutricional (con coincidencia exacta): la base, que
// sale de USDA, es más precisa (regla de CLAUDE.md, "Pipeline de registro").
type RecognizedFood struct {
	Name        string            // nombre genérico en inglés, para buscar en la base nutricional
	DisplayName string            // nombre para mostrar al usuario (español)
	Grams       float64           // porción estimada
	Confidence  float64           // 0–1
	Estimate    *NutrientEstimate // composición estimada por la IA; nil si no la dio
	// Unidades: lo que dijo o se ve ("2 huevos", "1 vaso", "3 cucharadas"). Quantity 0 =
	// solo gramos. Grams es siempre Quantity × UnitGrams (ver NormalizeUnits).
	Quantity  float64
	UnitOne   string // "huevo", "vaso", "cucharada"
	UnitMany  string // "huevos", "vasos", "cucharadas"
	UnitGrams float64
}

// HasUnits indica que la porción se cuenta en unidades (huevos, vasos…) y no solo en gramos.
func (r RecognizedFood) HasUnits() bool { return r.Quantity > 0 && r.UnitGrams > 0 && r.UnitOne != "" }

// maxQuantity acota las unidades de un ítem ("40 almendras" sí; "4000" es un error).
const maxQuantity = 200

// NormalizeUnits hace que los gramos sean las unidades por su peso. Si las unidades
// no tienen sentido, se descartan y quedan los gramos que dio la IA.
func (r *RecognizedFood) NormalizeUnits() {
	if r.Quantity <= 0 || r.Quantity > maxQuantity || r.UnitGrams <= 0 || r.UnitOne == "" ||
		r.Quantity*r.UnitGrams > MaxEntryGrams {
		r.Quantity, r.UnitOne, r.UnitMany, r.UnitGrams = 0, "", "", 0
		return
	}
	if r.UnitMany == "" {
		r.UnitMany = r.UnitOne
	}
	r.Grams = math.Max(1, math.Round(r.Quantity*r.UnitGrams))
}

// RecipeInfo indica que lo que se analizó es una receta (se cocinó para varias
// porciones): la app pregunta cuánto se comió antes de guardar.
type RecipeInfo struct {
	Name     string  // "Waffles proteicos"
	Servings float64 // porciones que rinde; 0 = no se sabe
}

// NutrientEstimate es la composición que estima la IA, en gramos cada 100 g del
// alimento tal como se sirve. La IA no da calorías: se calculan aquí (Atwater).
type NutrientEstimate struct {
	ProteinG float64
	CarbsG   float64 // totales, con la fibra
	FiberG   float64
	FatG     float64
	AlcoholG float64
}

// estimateSlackG: margen para el redondeo de la IA al sumar los macros de 100 g.
const estimateSlackG = 3

// Per100g valida la estimación y calcula las calorías con los factores de Atwater
// (proteína y carbohidratos digeribles 4 kcal/g, fibra 2, grasa 9, alcohol 7).
// Calcularlas aquí evita que la IA dé calorías que no cierran con sus macros.
func (e NutrientEstimate) Per100g() (Macros, error) {
	switch {
	case e.ProteinG < 0 || e.CarbsG < 0 || e.FiberG < 0 || e.FatG < 0 || e.AlcoholG < 0:
		return Macros{}, fmt.Errorf("%w: estimación con valores negativos", ErrInvalidMacros)
	case e.FiberG > e.CarbsG:
		return Macros{}, fmt.Errorf("%w: estimación con más fibra (%.1f g) que carbohidratos (%.1f g)", ErrInvalidMacros, e.FiberG, e.CarbsG)
	case e.ProteinG+e.CarbsG+e.FatG+e.AlcoholG > 100+estimateSlackG:
		return Macros{}, fmt.Errorf("%w: la estimación suma más de 100 g cada 100 g", ErrInvalidMacros)
	}
	kcal := 4*e.ProteinG + 4*(e.CarbsG-e.FiberG) + 2*e.FiberG + 9*e.FatG + 7*e.AlcoholG
	return Macros{Calories: kcal, ProteinG: e.ProteinG, CarbsG: e.CarbsG, FiberG: e.FiberG, FatG: e.FatG}, nil
}

// FoodRecognition es el resultado de analizar una foto o un texto transcrito.
// Items vacío significa "no se detectó comida" (y no se cobra el crédito).
type FoodRecognition struct {
	Items  []RecognizedFood
	Recipe *RecipeInfo // nil si no es una receta
}

// Validate comprueba el contrato de la respuesta de la IA.
func (r FoodRecognition) Validate() error {
	for i, it := range r.Items {
		switch {
		case strings.TrimSpace(it.Name) == "":
			return fmt.Errorf("%w: ítem %d sin nombre", ErrInvalidRecognition, i)
		case it.Grams <= 0 || it.Grams > 3000:
			return fmt.Errorf("%w: ítem %d (%s) con %.1f g", ErrInvalidRecognition, i, it.Name, it.Grams)
		case it.Confidence < 0 || it.Confidence > 1:
			return fmt.Errorf("%w: ítem %d (%s) con confianza %.2f", ErrInvalidRecognition, i, it.Name, it.Confidence)
		}
	}
	return nil
}

// IsEmpty indica que no se detectó comida.
func (r FoodRecognition) IsEmpty() bool { return len(r.Items) == 0 }

// TotalGrams suma los gramos de todos los ítems.
func (r FoodRecognition) TotalGrams() float64 {
	var total float64
	for _, it := range r.Items {
		total += it.Grams
	}
	return total
}

// NeedsReview indica si algún ítem está por debajo del umbral de confianza.
func (r FoodRecognition) NeedsReview() bool {
	for _, it := range r.Items {
		if it.Confidence < LowConfidenceThreshold {
			return true
		}
	}
	return false
}
