package ports

import (
	"context"

	"github.com/whiteowltech/owli/backend/internal/domain"
)

// ClosingView es lo que la app muestra en "qué comer ahora": cuánto falta para la
// próxima comida y los platos que lo cubren (domain.SuggestDishes).
type ClosingView struct {
	MealsLeft int
	Need      domain.Macros // para esta comida; en keto, CarbsG son carbos netos
	// Over: cuánto se pasó el día de cada macro; con grasa o carbos pasados, los
	// platos casi no suman de ese macro (domain.OverMacroAllowanceG).
	Over     domain.Macros
	NetCarbs bool
	// Closed: el día ya está cubierto (falta menos de domain.MinClosingKcal); no hay platos.
	Closed bool
	Dishes []domain.DishSuggestion
}

// SuggestionUseCase es el Meal Closing Engine (implementado en usecase.SuggestionService).
// No usa IA ni créditos: anotar un plato sugerido es gratis.
type SuggestionUseCase interface {
	Suggestions(ctx context.Context, userID domain.UserID) (ClosingView, error)
}
