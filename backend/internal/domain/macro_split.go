package domain

import (
	"fmt"
	"math"
	"time"
)

// MacroTargets son los gramos diarios objetivo de cada macro.
type MacroTargets struct {
	ProteinG float64
	CarbsG   float64 // en keto son carbohidratos netos (ver NetCarbs)
	FatG     float64
	NetCarbs bool // true en keto: el objetivo de carbos se compara con los netos
}

// ProteinPerKg son los gramos de proteína por kg de peso según el objetivo.
type ProteinPerKg struct {
	Cut, Maintain, Bulk, Recomp float64
}

func (p ProteinPerKg) forGoal(g Goal) (float64, bool) {
	switch g {
	case GoalCut:
		return p.Cut, true
	case GoalMaintain:
		return p.Maintain, true
	case GoalBulk:
		return p.Bulk, true
	case GoalRecomp:
		return p.Recomp, true
	}
	return 0, false
}

// MacroSplitPolicy define el reparto de macros.
//
// PROVISIONAL (aprobado el 3 de noviembre de 2026 como punto de partida):
// pendiente de validar con un profesional, igual que DefaultGoalPolicy.
type MacroSplitPolicy struct {
	Protein ProteinPerKg
	// MaxProteinShare acota la proteína en personas de mucho peso, donde los
	// g/kg sobre el peso total darían cifras excesivas (fracción de las kcal).
	MaxProteinShare float64
	// MinFatShare: la grasa nunca baja de esta fracción de las kcal.
	MinFatShare float64
	// KetoNetCarbsG: objetivo de carbos netos en keto (debajo del límite de 25 g).
	KetoNetCarbsG float64
}

// DefaultMacroSplit son los valores provisionales aprobados.
var DefaultMacroSplit = MacroSplitPolicy{
	Protein:         ProteinPerKg{Cut: 2.0, Maintain: 1.6, Bulk: 1.8, Recomp: 2.0},
	MaxProteinShare: 0.35,
	MinFatShare:     0.25,
	KetoNetCarbsG:   20,
}

// Kcal por gramo (Atwater).
const (
	kcalPerGProtein = 4.0
	kcalPerGCarbs   = 4.0
	kcalPerGFat     = 9.0
)

// TargetsFor reparte las calorías objetivo. Reglas:
//   - Proteína: g/kg según el objetivo, sin pasar de MaxProteinShare de las kcal.
//   - Keto: carbos netos fijos (KetoNetCarbsG) y la grasa completa el resto.
//   - Carnívoro: carbos en cero y la grasa completa el resto.
//   - Resto de dietas: grasa en MinFatShare de las kcal y los carbos completan
//     el resto (si no alcanza, carbos en cero y la grasa toma lo que queda).
func (p MacroSplitPolicy) TargetsFor(dailyKcal int, weightKg float64, goal Goal, diet DietType) (MacroTargets, error) {
	perKg, ok := p.Protein.forGoal(goal)
	if !ok {
		return MacroTargets{}, fmt.Errorf("%w: objetivo %q desconocido", ErrInvalidGoal, goal)
	}
	if _, err := ParseDiet(string(diet)); err != nil {
		return MacroTargets{}, err
	}
	if dailyKcal <= 0 || weightKg <= 0 {
		return MacroTargets{}, fmt.Errorf("%w: calorías o peso no válidos", ErrInvalidMacros)
	}
	kcal := float64(dailyKcal)
	protein := math.Min(perKg*weightKg, p.MaxProteinShare*kcal/kcalPerGProtein)
	rest := kcal - protein*kcalPerGProtein

	var t MacroTargets
	switch diet {
	case DietKeto:
		t = MacroTargets{ProteinG: protein, CarbsG: p.KetoNetCarbsG, NetCarbs: true}
		t.FatG = math.Max(0, (rest-t.CarbsG*kcalPerGCarbs)/kcalPerGFat)
	case DietCarnivore:
		t = MacroTargets{ProteinG: protein, FatG: math.Max(0, rest/kcalPerGFat)}
	default:
		fat := p.MinFatShare * kcal / kcalPerGFat
		carbs := (rest - fat*kcalPerGFat) / kcalPerGCarbs
		if carbs < 0 {
			carbs, fat = 0, math.Max(0, rest/kcalPerGFat)
		}
		t = MacroTargets{ProteinG: protein, CarbsG: carbs, FatG: fat}
	}
	t.ProteinG, t.CarbsG, t.FatG = math.Round(t.ProteinG), math.Round(t.CarbsG), math.Round(t.FatG)
	return t, nil
}

// Horario de referencia para el ritmo del día (ver DayProgress).
const (
	dayStartHour = 7
	dayEndHour   = 22
)

// DayProgress estima qué fracción del día "debería" estar comida a esta hora
// local: 0 antes de las 7, 1 después de las 22 y lineal en el medio. Es la
// entrada expectedProgress de EvaluateMood. Se usa la hora porque los alimentos
// todavía no se agrupan en comidas (desayuno, almuerzo…).
func DayProgress(local time.Time) float64 {
	h := float64(local.Hour()) + float64(local.Minute())/60
	return math.Max(0, math.Min(1, (h-dayStartHour)/(dayEndHour-dayStartHour)))
}
