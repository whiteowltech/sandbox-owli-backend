package domain

import (
	"errors"
	"fmt"
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
	Items []RecognizedFood
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
