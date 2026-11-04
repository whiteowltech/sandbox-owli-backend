package domain

import "testing"

func TestEvaluateMood(t *testing.T) {
	budget := MacroBudget{Calories: 2000, ProteinG: 150}
	tests := []struct {
		name     string
		consumed Macros
		progress float64
		want     OwliMood
	}{
		{"día cerrado", Macros{Calories: 1980, ProteinG: 148}, 1, MoodSatisfied},
		{"cerrado con proteína de sobra", Macros{Calories: 2090, ProteinG: 180}, 1, MoodSatisfied},
		{"calorías bien pero falta proteína", Macros{Calories: 2000, ProteinG: 100}, 1, MoodNeutral},
		{"déficit severo de proteína a media tarde", Macros{Calories: 1300, ProteinG: 30}, 0.66, MoodTired},
		{"poca proteína pero aún temprano", Macros{Calories: 300, ProteinG: 5}, 0.33, MoodNeutral},
		{"en ritmo", Macros{Calories: 1000, ProteinG: 80}, 0.5, MoodEnergetic},
		{"adelantado en calorías", Macros{Calories: 1500, ProteinG: 80}, 0.5, MoodNeutral},
		{"pasado de calorías", Macros{Calories: 2600, ProteinG: 160}, 1, MoodNeutral},
		{"sin registros al inicio del día", Macros{}, 0, MoodNeutral},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EvaluateMood(budget, tt.consumed, tt.progress); got != tt.want {
				t.Errorf("EvaluateMood = %q, se esperaba %q", got, tt.want)
			}
		})
	}
}

func TestEvaluateMoodWithoutBudget(t *testing.T) {
	if got := EvaluateMood(MacroBudget{}, Macros{Calories: 500}, 0.5); got != MoodNeutral {
		t.Errorf("sin presupuesto configurado = %q, se esperaba neutral", got)
	}
}
