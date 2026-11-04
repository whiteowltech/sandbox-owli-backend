package domain

import (
	"errors"
	"strings"
	"testing"
)

func validProfile() UserProfile {
	return UserProfile{
		Name:           "Leandro",
		Bio:            Biometrics{WeightKg: 80, HeightCm: 175, AgeYears: 30, Sex: SexMale, Activity: ActivityModerate},
		Goal:           GoalCut,
		Pace:           PaceSteady,
		TargetWeightKg: 74,
		Diet:           DietPaleo,
		Restrictions:   []Restriction{RestrictionGlutenFree},
		MealsPerDay:    4,
		Units:          UnitsMetric,
		Timezone:       "America/Argentina/Buenos_Aires",
	}
}

func TestUserProfileValidate(t *testing.T) {
	if err := validProfile().Validate(); err != nil {
		t.Fatalf("perfil válido rechazado: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*UserProfile)
		want   error
	}{
		{"nombre opcional", func(p *UserProfile) { p.Name = "" }, nil},
		{"nombre de 40 letras con acentos", func(p *UserProfile) { p.Name = strings.Repeat("á", 40) }, nil},
		{"nombre demasiado largo", func(p *UserProfile) { p.Name = strings.Repeat("a", 41) }, ErrInvalidProfile},
		{"menor de edad", func(p *UserProfile) { p.Bio.AgeYears = 17 }, ErrUnderage},
		{"objetivo desconocido", func(p *UserProfile) { p.Goal = "bajar" }, ErrInvalidGoal},
		{"dieta desconocida", func(p *UserProfile) { p.Diet = "frugivoro" }, ErrInvalidDiet},
		{"restricción desconocida", func(p *UserProfile) { p.Restrictions = []Restriction{"sin_todo"} }, ErrInvalidRestriction},
		{"restricción repetida", func(p *UserProfile) {
			p.Restrictions = []Restriction{RestrictionGlutenFree, RestrictionGlutenFree}
		}, ErrInvalidProfile},
		{"sin restricciones", func(p *UserProfile) { p.Restrictions = nil }, nil},
		{"OMAD", func(p *UserProfile) { p.MealsPerDay = 1 }, nil},
		{"6 comidas", func(p *UserProfile) { p.MealsPerDay = 6 }, ErrInvalidProfile},
		{"unidades desconocidas", func(p *UserProfile) { p.Units = "stones" }, ErrInvalidProfile},
		{"sin zona horaria", func(p *UserProfile) { p.Timezone = "" }, ErrInvalidProfile},
		{"zona horaria inventada", func(p *UserProfile) { p.Timezone = "Marte/Olympus" }, ErrInvalidProfile},
		{"zona horaria UTC", func(p *UserProfile) { p.Timezone = "UTC" }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validProfile()
			tt.mutate(&p)
			err := p.Validate()
			if tt.want == nil && err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, se esperaba %v", err, tt.want)
			}
		})
	}
}
