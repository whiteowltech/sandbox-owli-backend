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
	mealsLeft := domain.MealsLeft(profile.MealsPerDay, eatenAt)
	need := domain.MealNeed(float64(day.Plan.DailyTarget), day.Targets, day.Totals, mealsLeft)

	loc, err := time.LoadLocation(profile.Timezone)
	if err != nil {
		return ports.ClosingView{}, fmt.Errorf("zona horaria del perfil: %w", err)
	}
	dishes := domain.SuggestDishes(s.dishes, domain.ClosingRequest{
		Need: need, Diet: profile.Diet, Restrictions: profile.Restrictions,
		Breakfast: breakfastTime(s.now().In(loc)),
	})
	return ports.ClosingView{
		MealsLeft: mealsLeft, Need: need, NetCarbs: day.Targets.NetCarbs,
		Closed: need.Calories < domain.MinClosingKcal, Dishes: dishes,
	}, nil
}

// breakfastTime: desayuno (4 a 11 h) o merienda (16 a 19 h), en la hora local del
// usuario. A esas horas suben los platos de desayuno (yogur, tostadas, huevos).
func breakfastTime(local time.Time) bool {
	h := local.Hour()
	return (h >= 4 && h < 11) || (h >= 16 && h < 19)
}

var _ ports.SuggestionUseCase = (*SuggestionService)(nil)
