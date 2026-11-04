package ports

import (
	"context"
	"errors"
	"time"

	"github.com/whiteowltech/owli/backend/internal/domain"
)

// ErrProfileRequired: la operación necesita el perfil (zona horaria, objetivo)
// y el usuario todavía no lo subió.
var ErrProfileRequired = errors.New("primero hay que completar el perfil")

// FoodRepository es el catálogo de alimentos.
type FoodRepository interface {
	// Get devuelve ErrNotFound si el alimento no existe.
	Get(ctx context.Context, id domain.FoodID) (domain.Food, error)
	// Search busca por nombre o sinónimo, tolerando acentos y errores de tipeo.
	Search(ctx context.Context, query string, limit int) ([]domain.Food, error)
	// Match devuelve el alimento que mejor coincide con alguno de los nombres (lo que
	// identificó la IA: el genérico en inglés y el que se muestra en español).
	// ok = false si ninguno coincide lo suficiente: el usuario lo elige a mano.
	Match(ctx context.Context, names []string) (food domain.Food, ok bool, err error)
}

// LoggedEntry es una comida registrada con el nombre del alimento, para mostrarla.
type LoggedEntry struct {
	domain.MealEntry
	FoodNameES string
	FoodNameEN string
}

// EntryRepository guarda las comidas registradas. Toda operación filtra por el
// usuario autenticado.
type EntryRepository interface {
	Insert(ctx context.Context, userID domain.UserID, e domain.MealEntry) (string, error)
	// Delete devuelve ErrNotFound si la comida no existe o es de otro usuario.
	Delete(ctx context.Context, userID domain.UserID, entryID string) error
	// List devuelve las comidas con eaten_at en [from, to).
	List(ctx context.Context, userID domain.UserID, from, to time.Time) ([]LoggedEntry, error)
}

// DayView es el resumen de un día del usuario.
type DayView struct {
	Date    string // AAAA-MM-DD en la zona horaria del usuario
	Entries []LoggedEntry
	Totals  domain.Macros
	Micros  domain.MicroTotals
	Plan    domain.Plan
	Targets domain.MacroTargets
	// Mood es el estado de Owli según el ritmo del día (reglas de domain.EvaluateMood).
	Mood domain.OwliMood
}

// DiaryUseCase: registro manual y resumen del día (implementado en usecase.DiaryService).
// Registrar a mano es siempre gratis: no toca créditos.
type DiaryUseCase interface {
	SearchFoods(ctx context.Context, query string, limit int) ([]domain.Food, error)
	Log(ctx context.Context, userID domain.UserID, foodID domain.FoodID, grams float64, eatenAt time.Time, src domain.EntrySource) (LoggedEntry, error)
	Delete(ctx context.Context, userID domain.UserID, entryID string) error
	// Day: date vacío = hoy en la zona horaria del usuario.
	Day(ctx context.Context, userID domain.UserID, date string) (DayView, error)
}
