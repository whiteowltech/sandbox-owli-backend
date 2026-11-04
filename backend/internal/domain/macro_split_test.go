package domain

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"testing"
	"time"
)

// Mismos casos que app/src/app/core/nutrition/macros.spec.ts.
func TestTargetsForSharedCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/macro_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []struct {
			Name  string `json:"name"`
			Input struct {
				DailyKcal int      `json:"dailyKcal"`
				WeightKg  float64  `json:"weightKg"`
				Goal      Goal     `json:"goal"`
				Diet      DietType `json:"diet"`
			} `json:"input"`
			Want struct {
				ProteinG float64 `json:"proteinG"`
				CarbsG   float64 `json:"carbsG"`
				FatG     float64 `json:"fatG"`
				NetCarbs bool    `json:"netCarbs"`
			} `json:"want"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	for _, c := range file.Cases {
		t.Run(c.Name, func(t *testing.T) {
			in := c.Input
			got, err := DefaultMacroSplit.TargetsFor(in.DailyKcal, in.WeightKg, in.Goal, in.Diet)
			if err != nil {
				t.Fatal(err)
			}
			w := c.Want
			if got.ProteinG != w.ProteinG || got.CarbsG != w.CarbsG || got.FatG != w.FatG || got.NetCarbs != w.NetCarbs {
				t.Errorf("objetivos = %+v, se esperaba %+v", got, w)
			}
			// El reparto tiene que sumar las calorías (salvo el redondeo a gramos enteros).
			kcal := got.ProteinG*4 + got.CarbsG*4 + got.FatG*9
			if math.Abs(kcal-float64(in.DailyKcal)) > 9 {
				t.Errorf("los macros suman %.0f kcal, objetivo %d", kcal, in.DailyKcal)
			}
		})
	}
}

func TestTargetsForKetoStaysUnderLimit(t *testing.T) {
	got, _ := DefaultMacroSplit.TargetsFor(2000, 70, GoalMaintain, DietKeto)
	if got.CarbsG >= KetoNetCarbLimitG || !got.NetCarbs {
		t.Errorf("keto: %v g de carbos netos, el límite es < %v", got.CarbsG, KetoNetCarbLimitG)
	}
}

func TestTargetsForErrors(t *testing.T) {
	if _, err := DefaultMacroSplit.TargetsFor(2000, 70, "bajar", DietOmnivore); !errors.Is(err, ErrInvalidGoal) {
		t.Errorf("objetivo desconocido = %v", err)
	}
	if _, err := DefaultMacroSplit.TargetsFor(2000, 70, GoalCut, "frugivoro"); !errors.Is(err, ErrInvalidDiet) {
		t.Errorf("dieta desconocida = %v", err)
	}
	if _, err := DefaultMacroSplit.TargetsFor(0, 70, GoalCut, DietOmnivore); !errors.Is(err, ErrInvalidMacros) {
		t.Errorf("0 kcal = %v", err)
	}
}

func TestDayProgress(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 11, 3, h, m, 0, 0, time.UTC) }
	tests := []struct {
		t    time.Time
		want float64
	}{
		{at(6, 0), 0}, {at(7, 0), 0}, {at(14, 30), 0.5}, {at(22, 0), 1}, {at(23, 59), 1},
	}
	for _, tt := range tests {
		if got := DayProgress(tt.t); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("DayProgress(%s) = %v, se esperaba %v", tt.t.Format("15:04"), got, tt.want)
		}
	}
}
