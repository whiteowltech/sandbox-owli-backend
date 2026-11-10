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
	// Un alimento estimado por la IA es privado: para otro usuario no existe.
	if !food.VisibleTo(userID) {
		return ports.LoggedEntry{}, fmt.Errorf("buscando alimento: %w: %q", ports.ErrNotFound, foodID)
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
	plan, targets, err := planAndTargets(profile)
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

// planAndTargets: el objetivo diario y el reparto de macros del perfil.
func planAndTargets(p domain.UserProfile) (domain.Plan, domain.MacroTargets, error) {
	plan, err := domain.DefaultGoalPolicy.PlanFor(domain.PlanInput{Bio: p.Bio, Goal: p.Goal, Pace: p.Pace, TargetWeightKg: p.TargetWeightKg})
	if err != nil {
		return domain.Plan{}, domain.MacroTargets{}, err
	}
	targets, err := domain.DefaultMacroSplit.TargetsFor(plan.DailyTarget, p.Bio.WeightKg, p.Goal, p.Diet)
	return plan, targets, err
}

// weekDays es el largo del resumen semanal.
const weekDays = 7

func (s *DiaryService) Week(ctx context.Context, userID domain.UserID, end string) (ports.WeekView, error) {
	profile, err := s.profiles.Get(ctx, userID)
	if errors.Is(err, ports.ErrNotFound) {
		return ports.WeekView{}, ports.ErrProfileRequired
	}
	if err != nil {
		return ports.WeekView{}, fmt.Errorf("leyendo perfil: %w", err)
	}
	today, err := domain.Today(s.now(), profile.Timezone)
	if err != nil {
		return ports.WeekView{}, err
	}
	if end == "" {
		end = today
	}
	if end > today { // AAAA-MM-DD se ordena como texto
		return ports.WeekView{}, fmt.Errorf("%w: la semana no puede terminar en el futuro (%s)", domain.ErrInvalidProfile, end)
	}
	loc, _ := time.LoadLocation(profile.Timezone) // ya validada por Today
	endDay, err := time.ParseInLocation("2006-01-02", end, loc)
	if err != nil {
		return ports.WeekView{}, fmt.Errorf("%w: fecha %q (se espera AAAA-MM-DD)", domain.ErrInvalidProfile, end)
	}
	start := endDay.AddDate(0, 0, -(weekDays - 1)).Format("2006-01-02")
	from, _, err := domain.DayBounds(start, profile.Timezone)
	if err != nil {
		return ports.WeekView{}, err
	}
	_, to, err := domain.DayBounds(end, profile.Timezone)
	if err != nil {
		return ports.WeekView{}, err
	}
	entries, err := s.entries.List(ctx, userID, from, to)
	if err != nil {
		return ports.WeekView{}, fmt.Errorf("listando comidas: %w", err)
	}
	plan, targets, err := planAndTargets(profile)
	if err != nil {
		return ports.WeekView{}, err
	}

	// Una sola consulta para los 7 días; se agrupan por la fecha local del usuario.
	byDate := map[string][]domain.MealEntry{}
	for _, e := range entries {
		date := e.EatenAt.In(loc).Format("2006-01-02")
		byDate[date] = append(byDate[date], e.MealEntry)
	}
	budget := domain.MacroBudget{Calories: float64(plan.DailyTarget), ProteinG: targets.ProteinG}
	days := make([]domain.WeekDay, 0, weekDays)
	for i := range weekDays {
		date := endDay.AddDate(0, 0, i-(weekDays-1)).Format("2006-01-02")
		totals, _ := domain.DayTotals(byDate[date])
		progress := 1.0
		if date == today {
			progress = domain.DayProgress(s.now().In(loc))
		}
		days = append(days, domain.WeekDay{
			Date: date, Totals: totals, Entries: len(byDate[date]), InProgress: date == today,
			Mood: domain.EvaluateMood(budget, totals, progress),
		})
	}
	return ports.WeekView{
		Start: start, End: end, Today: today, Days: days, Plan: plan, Targets: targets,
		Review: domain.ReviewWeek(days, budget),
	}, nil
}

var _ ports.DiaryUseCase = (*DiaryService)(nil)
