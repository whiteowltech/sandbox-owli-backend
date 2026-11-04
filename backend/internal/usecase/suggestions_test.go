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

var (
	sChicken = domain.Food{ID: "pechuga-pollo", Per100g: domain.Macros{Calories: 165, ProteinG: 31, FatG: 3.6},
		Traits: domain.FoodTraits{AnimalOrigin: true, ContainsMeat: true, AllergensVerified: true}}
	sRice = domain.Food{ID: "arroz-blanco", Per100g: domain.Macros{Calories: 130, ProteinG: 2.7, CarbsG: 28.2, FatG: 0.3},
		Traits: domain.FoodTraits{PlantOrigin: true, ContainsGrain: true, AllergensVerified: true}}
	sTofu = domain.Food{ID: "tofu", Per100g: domain.Macros{Calories: 144, ProteinG: 17, CarbsG: 3, FiberG: 2, FatG: 9},
		Traits: domain.FoodTraits{PlantOrigin: true, ContainsLegume: true, AllergensVerified: true}}
	sOil = domain.Food{ID: "aceite-oliva", Per100g: domain.Macros{Calories: 884, FatG: 100},
		Traits: domain.FoodTraits{PlantOrigin: true, AllergensVerified: true}}
)

func testTemplates() []domain.DishTemplate {
	comp := func(f domain.Food, lo, hi float64) domain.DishComponent {
		return domain.DishComponent{Food: f, MinG: lo, MaxG: hi}
	}
	return []domain.DishTemplate{
		{ID: "pollo-arroz", NameES: "Pollo con arroz", Components: []domain.DishComponent{comp(sChicken, 60, 300), comp(sRice, 30, 350), comp(sOil, 3, 25)}},
		{ID: "tofu-arroz", NameES: "Tofu con arroz", Components: []domain.DishComponent{comp(sTofu, 60, 350), comp(sRice, 30, 350), comp(sOil, 3, 25)}},
	}
}

func newSuggestions(t *testing.T) (*SuggestionService, *DiaryService, *memory.Store, domain.UserID) {
	t.Helper()
	diary, store, u := newDiary(t)
	return NewSuggestionService(diary, store, testTemplates(), func() time.Time { return diaryNow }), diary, store, u
}

func TestSuggestionsForTheNextMeal(t *testing.T) {
	svc, _, store, u := newSuggestions(t)
	ctx := context.Background()
	if _, err := svc.Suggestions(ctx, u); !errors.Is(err, ports.ErrProfileRequired) {
		t.Fatalf("sin perfil: %v", err)
	}
	saveProfile(t, store, u) // omnívoro, 4 comidas al día

	got, err := svc.Suggestions(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if got.MealsLeft != 4 || got.Closed || len(got.Dishes) != 2 {
		t.Fatalf("sin nada anotado: %+v", got)
	}
	day, _ := svc.diary.Day(ctx, u, "")
	if want := float64(day.Plan.DailyTarget) / 4; got.Need.Calories != want {
		t.Errorf("lo que falta se reparte entre 4 comidas: %v, quería %v", got.Need.Calories, want)
	}
}

func TestSuggestionsAfterEating(t *testing.T) {
	svc, diary, store, u := newSuggestions(t)
	ctx := context.Background()
	saveProfile(t, store, u)

	// Una comida a las 13 h (hora de Buenos Aires): quedan 3.
	lunch := time.Date(2026, 11, 3, 16, 0, 0, 0, time.UTC)
	if _, err := diary.Log(ctx, u, "pechuga-pollo", 200, lunch, domain.SourceManual); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Suggestions(ctx, u)
	if got.MealsLeft != 3 {
		t.Errorf("comidas que quedan = %d", got.MealsLeft)
	}

	// Comer de más cierra el día: no se sugiere nada.
	if _, err := diary.Log(ctx, u, "arroz-blanco", 2500, lunch.Add(4*time.Hour), domain.SourceManual); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.Suggestions(ctx, u)
	if !got.Closed || len(got.Dishes) != 0 {
		t.Errorf("día cubierto: %+v", got)
	}
}

func TestSuggestionsRespectTheDiet(t *testing.T) {
	svc, _, store, u := newSuggestions(t)
	ctx := context.Background()
	saveProfile(t, store, u)
	p, _ := store.Get(ctx, u)
	p.Diet = domain.DietVegan
	if err := store.Upsert(ctx, u, p); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Suggestions(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Dishes) != 1 || got.Dishes[0].ID != "tofu-arroz" {
		t.Errorf("vegano: solo el tofu, %+v", got.Dishes)
	}
}

func TestBreakfastTime(t *testing.T) {
	for h, want := range map[int]bool{3: false, 4: true, 10: true, 11: false, 13: false, 16: true, 18: true, 19: false, 21: false} {
		if got := breakfastTime(time.Date(2026, 11, 3, h, 0, 0, 0, time.UTC)); got != want {
			t.Errorf("%d h: %v", h, got)
		}
	}
}
