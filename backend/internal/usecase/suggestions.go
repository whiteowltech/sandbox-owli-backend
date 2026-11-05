package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

// SuggestionService arma las sugerencias para la próxima comida a partir del día
// del usuario (lo comido, el objetivo y el reparto de macros) y de los platos curados.
type SuggestionService struct {
	diary    ports.DiaryUseCase
	profiles ports.ProfileRepository
	dishes   []domain.DishTemplate
	now      func() time.Time
}

func NewSuggestionService(diary ports.DiaryUseCase, profiles ports.ProfileRepository, dishes []domain.DishTemplate, now func() time.Time) *SuggestionService {
	return &SuggestionService{diary: diary, profiles: profiles, dishes: dishes, now: now}
}

func (s *SuggestionService) Suggestions(ctx context.Context, userID domain.UserID) (ports.ClosingView, error) {
	profile, err := s.profiles.Get(ctx, userID)
	if errors.Is(err, ports.ErrNotFound) {
		return ports.ClosingView{}, ports.ErrProfileRequired
	}
	if err != nil {
		return ports.ClosingView{}, fmt.Errorf("leyendo perfil: %w", err)
	}
	day, err := s.diary.Day(ctx, userID, "")
	if err != nil {
		return ports.ClosingView{}, err
	}
	eatenAt := make([]time.Time, len(day.Entries))
	for i, e := range day.Entries {
		eatenAt[i] = e.EatenAt
	}
	loc, err := time.LoadLocation(profile.Timezone)
	if err != nil {
		return ports.ClosingView{}, fmt.Errorf("zona horaria del perfil: %w", err)
	}
	next := domain.PlanNextMeal(profile.MealsPerDay, eatenAt, s.now().In(loc))
	kcal := float64(day.Plan.DailyTarget)
	need := domain.MealNeed(kcal, day.Targets, day.Totals, next.Left)
	over := domain.DayOver(kcal, day.Targets, day.Totals)

	dishes := domain.SuggestDishes(s.dishes, domain.ClosingRequest{
		Need: need, Over: over, Diet: profile.Diet, Restrictions: profile.Restrictions,
		Breakfast: next.Breakfast,
	})
	return ports.ClosingView{
		MealsLeft: next.Left, Need: need, Over: over, NetCarbs: day.Targets.NetCarbs,
		Closed: need.Calories < domain.MinClosingKcal, Dishes: dishes,
	}, nil
}

var _ ports.SuggestionUseCase = (*SuggestionService)(nil)
