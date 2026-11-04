package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

const (
	// MaxSearchResults acota la lista de resultados de búsqueda.
	MaxSearchResults = 30
	// minQueryRunes: con una sola letra la búsqueda no sirve y solo carga la base.
	minQueryRunes = 2
	// backdateWindow: se puede anotar algo que se olvidó hasta una semana atrás.
	backdateWindow = 7 * 24 * time.Hour
	// futureTolerance absorbe relojes de teléfonos levemente adelantados.
	futureTolerance = 5 * time.Minute
)

// DiaryService: registro manual (siempre gratis, no toca créditos) y resumen del día.
type DiaryService struct {
	foods    ports.FoodRepository
	entries  ports.EntryRepository
	profiles ports.ProfileRepository
	now      func() time.Time
}

func NewDiaryService(foods ports.FoodRepository, entries ports.EntryRepository, profiles ports.ProfileRepository, now func() time.Time) *DiaryService {
	return &DiaryService{foods: foods, entries: entries, profiles: profiles, now: now}
}

func (s *DiaryService) SearchFoods(ctx context.Context, query string, limit int) ([]domain.Food, error) {
	if len([]rune(domain.NormalizeSearch(query))) < minQueryRunes {
		return []domain.Food{}, nil
	}
	limit = min(max(limit, 1), MaxSearchResults)
	foods, err := s.foods.Search(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("buscando alimentos: %w", err)
	}
	return foods, nil
}

// Log registra una porción. eatenAt cero = ahora.
func (s *DiaryService) Log(ctx context.Context, userID domain.UserID, foodID domain.FoodID, grams float64, eatenAt time.Time, src domain.EntrySource) (ports.LoggedEntry, error) {
	now := s.now()
	if eatenAt.IsZero() {
		eatenAt = now
	}
	if eatenAt.After(now.Add(futureTolerance)) || eatenAt.Before(now.Add(-backdateWindow)) {
		return ports.LoggedEntry{}, fmt.Errorf("%w: la hora de la comida debe estar entre hace 7 días y ahora", domain.ErrInvalidMacros)
	}
	food, err := s.foods.Get(ctx, foodID)
	if err != nil {
		return ports.LoggedEntry{}, fmt.Errorf("buscando alimento: %w", err)
	}
	entry, err := domain.NewEntry(food, grams, eatenAt, src)
	if err != nil {
		return ports.LoggedEntry{}, err
	}
	id, err := s.entries.Insert(ctx, userID, entry)
	if err != nil {
		return ports.LoggedEntry{}, fmt.Errorf("guardando comida: %w", err)
	}
	entry.ID = id
	return ports.LoggedEntry{MealEntry: entry, FoodNameES: food.NameES, FoodNameEN: food.NameEN}, nil
}

func (s *DiaryService) Delete(ctx context.Context, userID domain.UserID, entryID string) error {
	if err := s.entries.Delete(ctx, userID, entryID); err != nil {
		return fmt.Errorf("borrando comida: %w", err)
	}
	return nil
}

// Day arma el resumen de un día. Necesita el perfil: su zona horaria define
// dónde corta el día y su objetivo, cuánto falta.
func (s *DiaryService) Day(ctx context.Context, userID domain.UserID, date string) (ports.DayView, error) {
	profile, err := s.profiles.Get(ctx, userID)
	if errors.Is(err, ports.ErrNotFound) {
		return ports.DayView{}, ports.ErrProfileRequired
	}
	if err != nil {
		return ports.DayView{}, fmt.Errorf("leyendo perfil: %w", err)
	}
	today, err := domain.Today(s.now(), profile.Timezone)
	if err != nil {
		return ports.DayView{}, err
	}
	if date == "" {
		date = today
	}
	from, to, err := domain.DayBounds(date, profile.Timezone)
	if err != nil {
		return ports.DayView{}, err
	}
	entries, err := s.entries.List(ctx, userID, from, to)
	if err != nil {
		return ports.DayView{}, fmt.Errorf("listando comidas: %w", err)
	}
	meals := make([]domain.MealEntry, len(entries))
	for i, e := range entries {
		meals[i] = e.MealEntry
	}
	totals, micros := domain.DayTotals(meals)
	plan, err := domain.DefaultGoalPolicy.PlanFor(domain.PlanInput{Bio: profile.Bio, Goal: profile.Goal, Pace: profile.Pace, TargetWeightKg: profile.TargetWeightKg})
	if err != nil {
		return ports.DayView{}, err
	}
	targets, err := domain.DefaultMacroSplit.TargetsFor(plan.DailyTarget, profile.Bio.WeightKg, profile.Goal, profile.Diet)
	if err != nil {
		return ports.DayView{}, err
	}
	// Ritmo esperado: hoy según la hora local; un día pasado ya terminó; uno futuro no empezó.
	var progress float64
	switch {
	case date < today:
		progress = 1
	case date == today:
		loc, _ := time.LoadLocation(profile.Timezone) // ya validada por Today
		progress = domain.DayProgress(s.now().In(loc))
	}
	mood := domain.EvaluateMood(domain.MacroBudget{Calories: float64(plan.DailyTarget), ProteinG: targets.ProteinG}, totals, progress)
	return ports.DayView{Date: date, Entries: entries, Totals: totals, Micros: micros, Plan: plan, Targets: targets, Mood: mood}, nil
}

var _ ports.DiaryUseCase = (*DiaryService)(nil)
