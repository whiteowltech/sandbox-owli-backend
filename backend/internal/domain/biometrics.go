package domain

import (
	"fmt"
	"math"
)

// MinUserAge es la edad mínima para usar Owli (validada en el onboarding).
const MinUserAge = 18

// Sex es el sexo biológico, necesario para la ecuación de Mifflin-St Jeor.
type Sex string

const (
	SexMale   Sex = "male"
	SexFemale Sex = "female"
)

// ActivityLevel es el nivel de actividad física declarado por el usuario.
type ActivityLevel string

const (
	ActivitySedentary  ActivityLevel = "sedentary"   // poco o ningún ejercicio
	ActivityLight      ActivityLevel = "light"       // 1–3 días por semana
	ActivityModerate   ActivityLevel = "moderate"    // 3–5 días por semana
	ActivityActive     ActivityLevel = "active"      // 6–7 días por semana
	ActivityVeryActive ActivityLevel = "very_active" // trabajo físico o doble sesión
)

// activityFactors son los multiplicadores estándar de BMR → TDEE.
var activityFactors = map[ActivityLevel]float64{
	ActivitySedentary:  1.2,
	ActivityLight:      1.375,
	ActivityModerate:   1.55,
	ActivityActive:     1.725,
	ActivityVeryActive: 1.9,
}

// Biometrics son los datos del paso 1 del onboarding. Siempre en sistema
// métrico; la conversión a imperial es responsabilidad de la UI.
type Biometrics struct {
	WeightKg float64
	HeightCm float64
	AgeYears int
	Sex      Sex
	Activity ActivityLevel
}

// Validate comprueba rangos fisiológicamente plausibles y la edad mínima.
func (b Biometrics) Validate() error {
	if b.AgeYears < MinUserAge {
		return ErrUnderage
	}
	switch {
	case b.AgeYears > 120:
		return fmt.Errorf("%w: edad %d fuera de rango", ErrInvalidBiometrics, b.AgeYears)
	case b.WeightKg < 30 || b.WeightKg > 350:
		return fmt.Errorf("%w: peso %.1f kg fuera de rango", ErrInvalidBiometrics, b.WeightKg)
	case b.HeightCm < 120 || b.HeightCm > 250:
		return fmt.Errorf("%w: altura %.1f cm fuera de rango", ErrInvalidBiometrics, b.HeightCm)
	case b.Sex != SexMale && b.Sex != SexFemale:
		return fmt.Errorf("%w: sexo %q desconocido", ErrInvalidBiometrics, b.Sex)
	}
	if _, ok := activityFactors[b.Activity]; !ok {
		return fmt.Errorf("%w: nivel de actividad %q desconocido", ErrInvalidBiometrics, b.Activity)
	}
	return nil
}

// BMR devuelve la tasa metabólica basal en kcal/día (Mifflin-St Jeor):
//
//	hombre: 10·peso + 6.25·altura − 5·edad + 5
//	mujer:  10·peso + 6.25·altura − 5·edad − 161
func (b Biometrics) BMR() (float64, error) {
	if err := b.Validate(); err != nil {
		return 0, err
	}
	base := 10*b.WeightKg + 6.25*b.HeightCm - 5*float64(b.AgeYears)
	if b.Sex == SexMale {
		return base + 5, nil
	}
	return base - 161, nil
}

// TDEE devuelve el gasto energético diario total en kcal/día, redondeado al
// entero más cercano: BMR × factor de actividad.
func (b Biometrics) TDEE() (int, error) {
	bmr, err := b.BMR()
	if err != nil {
		return 0, err
	}
	return int(math.Round(bmr * activityFactors[b.Activity])), nil
}

// MinSafeCalories es el piso calórico por debajo del cual Owli nunca fija un
// objetivo. Valores por defecto pendientes de validar con un profesional.
func MinSafeCalories(s Sex) int {
	if s == SexMale {
		return 1500
	}
	return 1200
}
