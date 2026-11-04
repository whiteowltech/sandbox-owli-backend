package domain

import (
	"fmt"
	"time"
	"unicode/utf8"
)

// UserID identifica a un usuario. Llega al dominio ya autenticado: nunca se toma
// de la URL ni del body (regla de autorización de CLAUDE.md).
type UserID string

// UnitSystem solo cambia cómo se muestran los datos: internamente todo es métrico.
type UnitSystem string

const (
	UnitsMetric   UnitSystem = "metric"
	UnitsImperial UnitSystem = "imperial"
)

// MaxNameLength es el largo máximo del nombre con el que Owli saluda.
const MaxNameLength = 40

// UserProfile es lo que el usuario define en el onboarding y puede editar en su cuenta.
type UserProfile struct {
	Name           string // opcional
	Bio            Biometrics
	Goal           Goal
	Pace           Pace
	TargetWeightKg float64
	Diet           DietType
	Restrictions   []Restriction
	MealsPerDay    int        // de 1 (OMAD) a 5
	Units          UnitSystem // preferencia de visualización
	// Timezone (IANA, p. ej. "America/Argentina/Buenos_Aires") define dónde corta
	// el "día" del usuario para los totales diarios.
	Timezone string
}

// Validate aplica todas las reglas del onboarding. El cliente valida lo mismo,
// pero el servidor nunca confía en él.
func (p UserProfile) Validate() error {
	if utf8.RuneCountInString(p.Name) > MaxNameLength {
		return fmt.Errorf("%w: el nombre supera %d caracteres", ErrInvalidProfile, MaxNameLength)
	}
	// PlanFor valida biometría (incluida la edad mínima), objetivo, ritmo y peso meta.
	if _, err := DefaultGoalPolicy.PlanFor(PlanInput{Bio: p.Bio, Goal: p.Goal, Pace: p.Pace, TargetWeightKg: p.TargetWeightKg}); err != nil {
		return err
	}
	if _, err := ParseDiet(string(p.Diet)); err != nil {
		return err
	}
	seen := make(map[Restriction]bool, len(p.Restrictions))
	for _, r := range p.Restrictions {
		if _, err := ParseRestriction(string(r)); err != nil {
			return err
		}
		if seen[r] {
			return fmt.Errorf("%w: restricción %q repetida", ErrInvalidProfile, r)
		}
		seen[r] = true
	}
	if p.MealsPerDay < 1 || p.MealsPerDay > 5 {
		return fmt.Errorf("%w: comidas al día %d fuera de 1 a 5", ErrInvalidProfile, p.MealsPerDay)
	}
	if p.Units != UnitsMetric && p.Units != UnitsImperial {
		return fmt.Errorf("%w: unidades %q desconocidas", ErrInvalidProfile, p.Units)
	}
	if p.Timezone == "" {
		return fmt.Errorf("%w: falta la zona horaria", ErrInvalidProfile)
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return fmt.Errorf("%w: zona horaria %q desconocida", ErrInvalidProfile, p.Timezone)
	}
	return nil
}
