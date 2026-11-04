package domain

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"testing"
)

// planCase replica el formato de testdata/plan_cases.json, que también usa la
// app (app/src/app/core/nutrition/plan.spec.ts) para garantizar las mismas reglas.
type planCase struct {
	Name  string `json:"name"`
	Input struct {
		Sex            Sex           `json:"sex"`
		AgeYears       int           `json:"ageYears"`
		HeightCm       float64       `json:"heightCm"`
		WeightKg       float64       `json:"weightKg"`
		Activity       ActivityLevel `json:"activity"`
		Goal           Goal          `json:"goal"`
		Pace           Pace          `json:"pace"`
		TargetWeightKg float64       `json:"targetWeightKg"`
	} `json:"input"`
	Want struct {
		TDEE                  int      `json:"tdee"`
		DailyTarget           int      `json:"dailyTarget"`
		MinSafe               int      `json:"minSafe"`
		Clamped               bool     `json:"clamped"`
		WeeklyRateKg          float64  `json:"weeklyRateKg"`
		WeeksToGoal           *float64 `json:"weeksToGoal"`
		WrongDirection        bool     `json:"wrongDirection"`
		TooSlow               bool     `json:"tooSlow"`
		TargetBelowHealthyBMI bool     `json:"targetBelowHealthyBmi"`
	} `json:"want"`
}

func TestPlanForSharedCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/plan_cases.json")
	if err != nil {
		t.Fatalf("leyendo casos: %v", err)
	}
	var file struct {
		Cases []planCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parseando casos: %v", err)
	}
	if len(file.Cases) == 0 {
		t.Fatal("no hay casos")
	}
	for _, c := range file.Cases {
		t.Run(c.Name, func(t *testing.T) {
			in := c.Input
			got, err := DefaultGoalPolicy.PlanFor(PlanInput{
				Bio:            Biometrics{WeightKg: in.WeightKg, HeightCm: in.HeightCm, AgeYears: in.AgeYears, Sex: in.Sex, Activity: in.Activity},
				Goal:           in.Goal,
				Pace:           in.Pace,
				TargetWeightKg: in.TargetWeightKg,
			})
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			w := c.Want
			if got.TDEE != w.TDEE || got.DailyTarget != w.DailyTarget || got.MinSafe != w.MinSafe || got.Clamped != w.Clamped {
				t.Errorf("TDEE/objetivo/mínimo/recorte = %d/%d/%d/%v, se esperaba %d/%d/%d/%v",
					got.TDEE, got.DailyTarget, got.MinSafe, got.Clamped, w.TDEE, w.DailyTarget, w.MinSafe, w.Clamped)
			}
			if math.Abs(got.WeeklyRateKg-w.WeeklyRateKg) > 1e-3 {
				t.Errorf("ritmo = %.4f kg/sem, se esperaba %.4f", got.WeeklyRateKg, w.WeeklyRateKg)
			}
			if got.WrongDirection != w.WrongDirection || got.TooSlow != w.TooSlow || got.TargetBelowHealthyBMI != w.TargetBelowHealthyBMI {
				t.Errorf("avisos (dirección, lento, IMC) = %v/%v/%v, se esperaba %v/%v/%v",
					got.WrongDirection, got.TooSlow, got.TargetBelowHealthyBMI, w.WrongDirection, w.TooSlow, w.TargetBelowHealthyBMI)
			}
			switch {
			case (got.WeeksToGoal == nil) != (w.WeeksToGoal == nil):
				t.Errorf("semanas = %v, se esperaba %v", got.WeeksToGoal, w.WeeksToGoal)
			case got.WeeksToGoal != nil && math.Abs(*got.WeeksToGoal-*w.WeeksToGoal) > 1e-3:
				t.Errorf("semanas = %.4f, se esperaba %.4f", *got.WeeksToGoal, *w.WeeksToGoal)
			}
		})
	}
}

func TestPlanForErrors(t *testing.T) {
	bio := Biometrics{WeightKg: 80, HeightCm: 175, AgeYears: 30, Sex: SexMale, Activity: ActivityModerate}
	tests := []struct {
		name string
		in   PlanInput
		want error
	}{
		{"objetivo desconocido", PlanInput{Bio: bio, Goal: "bajar", Pace: PaceSteady, TargetWeightKg: 75}, ErrInvalidGoal},
		{"ritmo desconocido", PlanInput{Bio: bio, Goal: GoalCut, Pace: "turbo", TargetWeightKg: 75}, ErrInvalidGoal},
		{"peso meta fuera de rango", PlanInput{Bio: bio, Goal: GoalCut, Pace: PaceSteady, TargetWeightKg: 20}, ErrInvalidBiometrics},
		{"menor de edad", PlanInput{Bio: Biometrics{80, 175, 17, SexMale, ActivityModerate}, Goal: GoalMaintain, TargetWeightKg: 80}, ErrUnderage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DefaultGoalPolicy.PlanFor(tt.in); !errors.Is(err, tt.want) {
				t.Errorf("error = %v, se esperaba %v", err, tt.want)
			}
		})
	}
}

func TestGoalPolicyValidate(t *testing.T) {
	if err := DefaultGoalPolicy.Validate(); err != nil {
		t.Errorf("la política por defecto debería ser válida: %v", err)
	}
	bad := DefaultGoalPolicy
	bad.Cut.Fast = 0
	if err := bad.Validate(); !errors.Is(err, ErrInvalidGoal) {
		t.Errorf("ritmo 0 debería fallar, got %v", err)
	}
}
