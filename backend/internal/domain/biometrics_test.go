package domain

import (
	"errors"
	"testing"
)

func TestTDEE(t *testing.T) {
	tests := []struct {
		name string
		b    Biometrics
		want int
	}{
		// 10·80 + 6.25·180 − 5·30 + 5 = 1780 → ×1.55 = 2759
		{"hombre moderado", Biometrics{80, 180, 30, SexMale, ActivityModerate}, 2759},
		// 10·60 + 6.25·165 − 5·25 − 161 = 1345.25 → ×1.2 = 1614.3
		{"mujer sedentaria", Biometrics{60, 165, 25, SexFemale, ActivitySedentary}, 1614},
		// 10·95 + 6.25·175 − 5·45 + 5 = 1823.75 → ×1.9 = 3465.1
		{"hombre muy activo", Biometrics{95, 175, 45, SexMale, ActivityVeryActive}, 3465},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.b.TDEE()
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if got != tt.want {
				t.Errorf("TDEE = %d, se esperaba %d", got, tt.want)
			}
		})
	}
}

func TestBiometricsValidate(t *testing.T) {
	valid := Biometrics{70, 170, 30, SexFemale, ActivityLight}
	tests := []struct {
		name   string
		mutate func(*Biometrics)
		want   error
	}{
		{"válido", func(*Biometrics) {}, nil},
		{"menor de edad", func(b *Biometrics) { b.AgeYears = 17 }, ErrUnderage},
		{"peso imposible", func(b *Biometrics) { b.WeightKg = 10 }, ErrInvalidBiometrics},
		{"altura imposible", func(b *Biometrics) { b.HeightCm = 300 }, ErrInvalidBiometrics},
		{"sexo desconocido", func(b *Biometrics) { b.Sex = "x" }, ErrInvalidBiometrics},
		{"actividad desconocida", func(b *Biometrics) { b.Activity = "extreme" }, ErrInvalidBiometrics},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := valid
			tt.mutate(&b)
			if err := b.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, se esperaba %v", err, tt.want)
			}
		})
	}
}
