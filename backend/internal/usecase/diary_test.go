package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/whiteowltech/owli/backend/internal/adapters/memory"
	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

// 3 nov 2026, 20:00 en Buenos Aires (23:00 UTC).
var diaryNow = time.Date(2026, 11, 3, 23, 0, 0, 0, time.UTC)

func newDiary(t *testing.T) (*DiaryService, *memory.Store, domain.UserID) {
	t.Helper()
	ctx := context.Background()
	store := memory.NewStore()
	store.AddFood(domain.Food{ID: "pechuga-pollo", NameES: "Pechuga de pollo asada", NameEN: "Roasted chicken breast",
		Per100g: domain.Macros{Calories: 165, ProteinG: 31, FatG: 3.6}})
	store.AddFood(domain.Food{ID: "arroz-blanco", NameES: "Arroz blanco cocido", NameEN: "Cooked white rice",
		Per100g: domain.Macros{Calories: 130, ProteinG: 2.7, CarbsG: 28.2, FatG: 0.3}})
	u, _ := store.CreateUser(ctx)
	svc := NewDiaryService(store.FoodsRepo(), store.EntriesRepo(), store, func() time.Time { return diaryNow })
	return svc, store, u
}

func saveProfile(t *testing.T, store *memory.Store, u domain.UserID) {
	t.Helper()
	p := domain.UserProfile{
		Bio:  domain.Biometrics{WeightKg: 80, HeightCm: 175, AgeYears: 30, Sex: domain.SexMale, Activity: domain.ActivityModerate},
		Goal: domain.GoalCut, Pace: domain.PaceSteady, TargetWeightKg: 74, Diet: domain.DietOmnivore,
		MealsPerDay: 4, Units: domain.UnitsMetric, Timezone: "America/Argentina/Buenos_Aires",
	}
	if err := store.Upsert(context.Background(), u, p); err != nil {
		t.Fatal(err)
	}
}

func TestSearchFoods(t *testing.T) {
	svc, _, u := newDiary(t)
	ctx := context.Background()
	got, _ := svc.SearchFoods(ctx, u, "pollo", 10)
	if len(got) != 1 || got[0].ID != "pechuga-pollo" {
		t.Errorf("pollo = %v", got)
	}
	if got, _ := svc.SearchFoods(ctx, u, "p", 10); len(got) != 0 {
		t.Error("una sola letra no busca")
	}
}

func TestLogAndDay(t *testing.T) {
	svc, store, u := newDiary(t)
	ctx := context.Background()

	if _, err := svc.Day(ctx, u, ""); !errors.Is(err, ports.ErrProfileRequired) {
		t.Fatalf("sin perfil = %v", err)
	}
	saveProfile(t, store, u)

	lunch, err := svc.Log(ctx, u, "pechuga-pollo", 200, time.Time{}, domain.SourceManual)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if lunch.ID == "" || lunch.Macros.Calories != 330 || lunch.FoodNameES != "Pechuga de pollo asada" {
		t.Errorf("comida = %+v", lunch)
	}
	// 22:30 en Buenos Aires = 01:30 UTC del día siguiente, pero sigue siendo "hoy" para el usuario.
	late := time.Date(2026, 11, 4, 1, 30, 0, 0, time.UTC)
	svcLate := NewDiaryService(store.FoodsRepo(), store.EntriesRepo(), store, func() time.Time { return late })
	if _, err := svcLate.Log(ctx, u, "arroz-blanco", 150, late, domain.SourceManual); err != nil {
		t.Fatal(err)
	}
	// Ayer: no cuenta en el día de hoy.
	if _, err := svc.Log(ctx, u, "arroz-blanco", 100, diaryNow.Add(-24*time.Hour), domain.SourceManual); err != nil {
		t.Fatal(err)
	}

	day, err := svcLate.Day(ctx, u, "")
	if err != nil {
		t.Fatalf("Day: %v", err)
	}
	if day.Date != "2026-11-03" || len(day.Entries) != 2 {
		t.Fatalf("día %s con %d comidas, se esperaba 2026-11-03 con 2", day.Date, len(day.Entries))
	}
	if day.Totals.Calories != 330+195 || day.Plan.DailyTarget != 2161 {
		t.Errorf("total %v kcal, objetivo %d", day.Totals.Calories, day.Plan.DailyTarget)
	}
	// Mismo caso que testdata/macro_cases.json (omnívoro, perder grasa, 80 kg).
	if day.Targets.ProteinG != 160 || day.Targets.CarbsG != 245 || day.Targets.FatG != 60 {
		t.Errorf("objetivos = %+v", day.Targets)
	}
	// 22:30: el día ya debería estar completo y la proteína (66 g de 160) va muy atrás.
	if day.Mood != domain.MoodTired {
		t.Errorf("ánimo = %s, se esperaba tired", day.Mood)
	}
	yesterday, _ := svc.Day(ctx, u, "2026-11-02")
	if len(yesterday.Entries) != 1 {
		t.Errorf("ayer = %d comidas", len(yesterday.Entries))
	}

	if err := svc.Delete(ctx, u, lunch.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, u, lunch.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("borrar dos veces = %v", err)
	}
}

func TestLogValidation(t *testing.T) {
	svc, _, u := newDiary(t)
	ctx := context.Background()
	if _, err := svc.Log(ctx, u, "no-existe", 100, time.Time{}, domain.SourceManual); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("alimento inexistente = %v", err)
	}
	if _, err := svc.Log(ctx, u, "pechuga-pollo", 0, time.Time{}, domain.SourceManual); !errors.Is(err, domain.ErrInvalidMacros) {
		t.Errorf("0 g = %v", err)
	}
	if _, err := svc.Log(ctx, u, "pechuga-pollo", 100, diaryNow.Add(time.Hour), domain.SourceManual); !errors.Is(err, domain.ErrInvalidMacros) {
		t.Errorf("comida en el futuro = %v", err)
	}
	if _, err := svc.Log(ctx, u, "pechuga-pollo", 100, diaryNow.Add(-8*24*time.Hour), domain.SourceManual); !errors.Is(err, domain.ErrInvalidMacros) {
		t.Errorf("hace 8 días = %v", err)
	}
}

func TestUsersCannotTouchOthersEntries(t *testing.T) {
	svc, store, alice := newDiary(t)
	ctx := context.Background()
	bob, _ := store.CreateUser(ctx)
	e, _ := svc.Log(ctx, alice, "pechuga-pollo", 100, time.Time{}, domain.SourceManual)
	if err := svc.Delete(ctx, bob, e.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("Bob borrando la comida de Alice = %v", err)
	}
}

func TestMoodFollowsTheDay(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	store.AddFood(domain.Food{ID: "pechuga-pollo", NameES: "Pechuga", NameEN: "Chicken",
		Per100g: domain.Macros{Calories: 165, ProteinG: 31, FatG: 3.6}})
	u, _ := store.CreateUser(ctx)
	saveProfile(t, store, u)
	// 10:00 en Buenos Aires.
	morning := time.Date(2026, 11, 3, 13, 0, 0, 0, time.UTC)
	svc := NewDiaryService(store.FoodsRepo(), store.EntriesRepo(), store, func() time.Time { return morning })

	day, _ := svc.Day(ctx, u, "")
	if day.Mood != domain.MoodNeutral {
		t.Errorf("sin comer a la mañana = %s, se esperaba neutral", day.Mood)
	}
	// Desayuno con buena proteína: ~ritmo esperado (20 %) → enérgico.
	if _, err := svc.Log(ctx, u, "pechuga-pollo", 150, morning, domain.SourceManual); err != nil {
		t.Fatal(err)
	}
	if day, _ = svc.Day(ctx, u, ""); day.Mood != domain.MoodEnergetic {
		t.Errorf("buen desayuno = %s, se esperaba energetic", day.Mood)
	}
	// Un día pasado sin comidas se evalúa completo: proteína muy atrás → cansado.
	if past, _ := svc.Day(ctx, u, "2026-11-01"); past.Mood != domain.MoodTired {
		t.Errorf("día pasado vacío = %s, se esperaba tired", past.Mood)
	}
}
