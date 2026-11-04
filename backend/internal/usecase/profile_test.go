package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/whiteowltech/owli/backend/internal/adapters/memory"
	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

func TestProfileSaveAndGet(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewStore()
	svc := NewProfileService(repo)
	u1, _ := repo.CreateUser(ctx)

	if _, err := svc.Get(ctx, u1); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("sin perfil = %v, se esperaba ErrNotFound", err)
	}

	p := domain.UserProfile{
		Bio:            domain.Biometrics{WeightKg: 80, HeightCm: 175, AgeYears: 30, Sex: domain.SexMale, Activity: domain.ActivityModerate},
		Goal:           domain.GoalCut,
		Pace:           domain.PaceSteady,
		TargetWeightKg: 74,
		Diet:           domain.DietOmnivore,
		MealsPerDay:    4,
		Units:          domain.UnitsMetric,
		Timezone:       "UTC",
	}
	plan, err := svc.Save(ctx, u1, p)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if plan.DailyTarget != 2161 { // mismo caso que testdata/plan_cases.json
		t.Errorf("objetivo = %d, se esperaba 2161", plan.DailyTarget)
	}
	got, _ := svc.Get(ctx, u1)
	if got.Restrictions == nil {
		t.Error("las restricciones vacías se guardan como lista vacía, no nil")
	}

	p.Bio.AgeYears = 16
	if _, err := svc.Save(ctx, u1, p); !errors.Is(err, domain.ErrUnderage) {
		t.Errorf("perfil de menor = %v, se esperaba ErrUnderage", err)
	}
	if stored, _ := svc.Get(ctx, u1); stored.Bio.AgeYears != 30 {
		t.Error("un perfil inválido no debe pisar el guardado")
	}
}
