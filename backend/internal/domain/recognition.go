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

// RecognizedFood es un alimento identificado por la IA. Solo contiene
// identificación y gramos: los macros se calculan después con la base
// nutricional (ver ForGrams), nunca los aporta la IA.
type RecognizedFood struct {
	Name        string  // nombre genérico en inglés, para buscar en la base nutricional
	DisplayName string  // nombre para mostrar al usuario (español)
	Grams       float64 // porción estimada
	Confidence  float64 // 0–1
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
