package domain

import (
	"fmt"
	"math"
)

// Goal es el objetivo elegido en el paso 2 del onboarding.
type Goal string

const (
	GoalCut      Goal = "cut"      // perder grasa
	GoalMaintain Goal = "maintain" // mantener
	GoalBulk     Goal = "bulk"     // ganar músculo
	GoalRecomp   Goal = "recomp"   // recomposición
)

// Pace es el ritmo elegido para perder o ganar peso.
type Pace string

const (
	PaceSlow   Pace = "slow"
	PaceSteady Pace = "steady"
	PaceFast   Pace = "fast"
)

// MinHealthyBMI es el límite inferior del rango de IMC saludable (OMS). Un peso
// meta por debajo se señala al usuario; no se usa para nada más.
const MinHealthyBMI = 18.5

// PaceRates son los kg por semana de cada ritmo.
type PaceRates struct {
	Slow, Steady, Fast float64
}

func (r PaceRates) forPace(p Pace) (float64, bool) {
	switch p {
	case PaceSlow:
		return r.Slow, true
	case PaceSteady:
		return r.Steady, true
	case PaceFast:
		return r.Fast, true
	}
	return 0, false
}

// GoalPolicy define cómo se traduce un objetivo y un ritmo en calorías.
//
// PROVISIONAL: los valores por defecto (DefaultGoalPolicy) están pendientes de
// validar con un profesional (ver pendientes.md). Viven en un struct, y no
// sueltos en el código, para poder cambiarlos desde la configuración.
type GoalPolicy struct {
	// KcalPerKgWeek: kcal diarias de déficit o superávit por cada kg/semana
	// (~7700 kcal por kg de grasa ÷ 7 días).
	KcalPerKgWeek float64
	Cut           PaceRates
	Bulk          PaceRates
	// RecompDeficit: déficit leve y fijo de la recomposición, en kcal/día.
	RecompDeficit int
	// TooSlowKgWeek: por debajo de este ritmo real (cuando el mínimo seguro
	// recorta el déficit) no se muestra una fecha: estaría a años vista.
	TooSlowKgWeek float64
}

// DefaultGoalPolicy son los valores provisionales del prototipo v5.
var DefaultGoalPolicy = GoalPolicy{
	KcalPerKgWeek: 1100,
	Cut:           PaceRates{Slow: 0.25, Steady: 0.5, Fast: 0.75},
	Bulk:          PaceRates{Slow: 0.15, Steady: 0.25, Fast: 0.4},
	RecompDeficit: 200,
	TooSlowKgWeek: 0.1,
}

// Validate comprueba que la política sea coherente.
func (p GoalPolicy) Validate() error {
	rates := []float64{p.Cut.Slow, p.Cut.Steady, p.Cut.Fast, p.Bulk.Slow, p.Bulk.Steady, p.Bulk.Fast}
	for _, r := range rates {
		if r <= 0 {
			return fmt.Errorf("%w: los ritmos deben ser positivos", ErrInvalidGoal)
		}
	}
	if p.KcalPerKgWeek <= 0 || p.RecompDeficit < 0 || p.TooSlowKgWeek < 0 {
		return fmt.Errorf("%w: política de objetivos inválida", ErrInvalidGoal)
	}
	return nil
}

// PlanInput es lo que el usuario eligió en el onboarding.
type PlanInput struct {
	Bio            Biometrics
	Goal           Goal
	Pace           Pace // se ignora en mantener y recomposición
	TargetWeightKg float64
}

// Plan es el objetivo diario calculado y todo lo que la UI necesita explicar.
type Plan struct {
	TDEE        int // gasto diario estimado
	DailyTarget int // calorías objetivo (nunca por debajo de MinSafe)
	MinSafe     int
	// Clamped indica que el objetivo se subió al mínimo seguro.
	Clamped bool
	// WeeklyRateKg es el ritmo real, ya contando el recorte del mínimo seguro.
	WeeklyRateKg float64
	// WeeksToGoal es la estimación hasta el peso meta. nil cuando no aplica:
	// mantener, recomposición, peso meta en la dirección equivocada o ritmo
	// demasiado lento.
	WeeksToGoal *float64
	// WrongDirection: perder grasa con un peso meta mayor al actual (o al revés).
	WrongDirection bool
	// TooSlow: el mínimo seguro deja un ritmo casi nulo. La UI sugiere moverse
	// más en vez de mostrar una fecha a años vista.
	TooSlow bool
	// TargetBelowHealthyBMI: el peso meta queda por debajo del IMC 18,5.
	TargetBelowHealthyBMI bool
}

// PlanFor calcula el objetivo diario. Reglas:
//   - Perder grasa: TDEE − ritmo × KcalPerKgWeek. Ganar músculo: TDEE + ritmo × KcalPerKgWeek.
//   - Recomposición: TDEE − RecompDeficit. Mantener: TDEE.
//   - El objetivo nunca baja de MinSafeCalories; si se recorta, el ritmo real
//     (y por lo tanto la fecha) se recalcula con el déficit efectivo.
func (p GoalPolicy) PlanFor(in PlanInput) (Plan, error) {
	tdee, err := in.Bio.TDEE()
	if err != nil {
		return Plan{}, err
	}
	if in.TargetWeightKg < 30 || in.TargetWeightKg > 350 {
		return Plan{}, fmt.Errorf("%w: peso meta %.1f kg fuera de rango", ErrInvalidBiometrics, in.TargetWeightKg)
	}

	var delta float64
	switch in.Goal {
	case GoalMaintain:
	case GoalRecomp:
		delta = -float64(p.RecompDeficit)
	case GoalCut, GoalBulk:
		rates := p.Cut
		sign := -1.0
		if in.Goal == GoalBulk {
			rates, sign = p.Bulk, 1
		}
		rate, ok := rates.forPace(in.Pace)
		if !ok {
			return Plan{}, fmt.Errorf("%w: ritmo %q desconocido", ErrInvalidGoal, in.Pace)
		}
		delta = sign * rate * p.KcalPerKgWeek
	default:
		return Plan{}, fmt.Errorf("%w: objetivo %q desconocido", ErrInvalidGoal, in.Goal)
	}

	minSafe := MinSafeCalories(in.Bio.Sex)
	raw := int(math.Round(float64(tdee) + delta))
	plan := Plan{TDEE: tdee, DailyTarget: max(raw, minSafe), MinSafe: minSafe, Clamped: raw < minSafe}
	plan.WeeklyRateKg = math.Abs(float64(plan.DailyTarget-tdee)) / p.KcalPerKgWeek

	heightM := in.Bio.HeightCm / 100
	plan.TargetBelowHealthyBMI = in.TargetWeightKg < MinHealthyBMI*heightM*heightM

	if in.Goal != GoalCut && in.Goal != GoalBulk {
		return plan, nil
	}
	diff := in.TargetWeightKg - in.Bio.WeightKg
	if (in.Goal == GoalCut && diff >= 0) || (in.Goal == GoalBulk && diff <= 0) {
		plan.WrongDirection = true
		return plan, nil
	}
	if plan.WeeklyRateKg < p.TooSlowKgWeek {
		plan.TooSlow = true
		return plan, nil
	}
	weeks := math.Abs(diff) / plan.WeeklyRateKg
	plan.WeeksToGoal = &weeks
	return plan, nil
}
