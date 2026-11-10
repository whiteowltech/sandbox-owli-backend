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
	// Get devuelve ErrNotFound si el alimento no existe. Incluye los alimentos
	// propios (con dueño): quien llama comprueba que el usuario pueda verlo.
	Get(ctx context.Context, id domain.FoodID) (domain.Food, error)
	// Search busca por nombre o sinónimo, tolerando acentos y errores de tipeo, en
	// los alimentos propios del usuario (primero) y en el catálogo.
	Search(ctx context.Context, owner domain.UserID, query string, limit int) ([]domain.Food, error)
	// Match busca un alimento cuyo nombre o sinónimo sea el mismo que uno de los
	// nombres (lo que identificó la IA: el genérico en inglés y el que se muestra en
	// español) según domain.NamesMatch: primero los propios del usuario (sus recetas
	// y lo que ya estimó la IA), después el catálogo. Una coincidencia solo parecida
	// reemplazaba lo que vio la IA por otro alimento ("estofado" → "bife").
	// ok = false si ninguno coincide: se usa la estimación de la IA.
	Match(ctx context.Context, owner domain.UserID, names []string) (food domain.Food, ok bool, err error)
	// SaveEstimate guarda un alimento estimado por la IA (domain.NewEstimatedFood) si
	// todavía no existe y devuelve el guardado: si ya estaba, gana el anterior, para
	// que el mismo alimento tenga siempre los mismos valores.
	SaveEstimate(ctx context.Context, f domain.Food) (domain.Food, error)
	// SaveRecipe guarda o actualiza una receta propia (domain.NewUserRecipe).
	SaveRecipe(ctx context.Context, f domain.Food) (domain.Food, error)
	// RecordMiss cuenta un alimento que la IA identificó y no está en el catálogo,
	// para saber cuáles sumar con datos de USDA. Solo el nombre: sin usuario.
	RecordMiss(ctx context.Context, name string) error
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

// WeekView es el resumen de 7 días para "Tu semana".
//
// El objetivo es el del perfil actual: si el usuario cambió su plan, los días
// anteriores se comparan con el nuevo (todavía no se guarda el historial del plan).
type WeekView struct {
	Start, End string // AAAA-MM-DD en la zona horaria del usuario
	Today      string
	Days       []domain.WeekDay
	Plan       domain.Plan
	Targets    domain.MacroTargets
	Review     domain.WeekReview
}

// DiaryUseCase: registro manual y resumen del día (implementado en usecase.DiaryService).
// Registrar a mano es siempre gratis: no toca créditos.
// RecipeInput es una receta para guardar como plato propio y anotar lo que se comió.
type RecipeInput struct {
	Name        string
	Servings    float64 // 0 = no se sabe
	Ingredients []RecipeIngredientInput
	// EatenGrams: cuánto se comió de toda la receta (la app pregunta "¿cuánto comiste?").
	EatenGrams float64
	EatenAt    time.Time // cero = ahora
	Source     domain.EntrySource
}

// RecipeIngredientInput es un ingrediente: un alimento que el usuario puede ver y sus gramos.
type RecipeIngredientInput struct {
	FoodID domain.FoodID
	Grams  float64
}

// MaxRecipeIngredients acota los ingredientes de una receta propia.
const MaxRecipeIngredients = 30

type DiaryUseCase interface {
	// SearchFoods busca en los alimentos propios del usuario (primero) y en el catálogo.
	SearchFoods(ctx context.Context, userID domain.UserID, query string, limit int) ([]domain.Food, error)
	// LogRecipe guarda (o actualiza) la receta como plato propio y anota la porción comida.
	LogRecipe(ctx context.Context, userID domain.UserID, in RecipeInput) (domain.Food, LoggedEntry, error)
	Log(ctx context.Context, userID domain.UserID, foodID domain.FoodID, grams float64, eatenAt time.Time, src domain.EntrySource) (LoggedEntry, error)
	Delete(ctx context.Context, userID domain.UserID, entryID string) error
	// Day: date vacío = hoy en la zona horaria del usuario.
	Day(ctx context.Context, userID domain.UserID, date string) (DayView, error)
	// Week: los 7 días que terminan en end (vacío = hoy). No puede ser un día futuro.
	Week(ctx context.Context, userID domain.UserID, end string) (WeekView, error)
}
