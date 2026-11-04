package domain

import "math"

// Tolerancias para considerar "cerrado" el día. Las comparte el Meal Closing
// Engine: una sugerencia es válida si deja el día dentro de estos márgenes.
const (
	ClosingCalorieTolerance  = 0.05 // ±5 % de las calorías objetivo
	ClosingProteinToleranceG = 5.0  // ±5 g de proteína
)

// MacroBudget es lo que mira el estado de ánimo: calorías y proteína del día.
// El reparto completo de macros está en MacroSplitPolicy (macro_split.go).
type MacroBudget struct {
	Calories float64
	ProteinG float64
}

// OwliMood es el estado emocional del búho en la cabecera del dashboard.
type OwliMood string

const (
	MoodNeutral   OwliMood = "neutral"   // sin señal clara
	MoodEnergetic OwliMood = "energetic" // va en ritmo con calorías y proteína
	MoodTired     OwliMood = "tired"     // déficit severo de proteína para la altura del día
	MoodSatisfied OwliMood = "satisfied" // balance calórico y proteico cerrado
)

// EvaluateMood decide el estado de Owli a partir del progreso del día.
//
// expectedProgress (0–1) es qué fracción del día "debería" estar cubierta: la
// calcula la capa de aplicación (p. ej. comidas registradas / comidas por día,
// o la hora local del usuario). Mantenerlo como entrada deja la función pura.
//
// Reglas, en orden de prioridad:
//  1. Satisfecho: calorías dentro de ±5 % y proteína a no más de 5 g del objetivo (o por encima).
//  2. Cansado: pasada la mitad del día, la proteína va por debajo de la mitad del ritmo esperado.
//  3. Enérgico: proteína al menos al 80 % del ritmo esperado sin adelantarse más de 15 puntos en calorías.
//  4. Neutral en cualquier otro caso (incluido pasarse de calorías).
func EvaluateMood(budget MacroBudget, consumed Macros, expectedProgress float64) OwliMood {
	if budget.Calories <= 0 || budget.ProteinG <= 0 {
		return MoodNeutral
	}
	progress := math.Max(0, math.Min(1, expectedProgress))
	calRatio := consumed.Calories / budget.Calories
	protRatio := consumed.ProteinG / budget.ProteinG

	if math.Abs(calRatio-1) <= ClosingCalorieTolerance &&
		consumed.ProteinG >= budget.ProteinG-ClosingProteinToleranceG {
		return MoodSatisfied
	}
	if progress >= 0.5 && protRatio < progress*0.5 {
		return MoodTired
	}
	if progress > 0 && protRatio >= progress*0.8 && calRatio <= progress+0.15 {
		return MoodEnergetic
	}
	return MoodNeutral
}
