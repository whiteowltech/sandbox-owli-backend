package httpapi

import (
	"net/http"

	"github.com/whiteowltech/owli/backend/internal/ports"
)

type suggestionHandlers struct {
	uc ports.SuggestionUseCase
	h  *handlers
}

type portionDTO struct {
	Grams float64 `json:"grams"`
	Food  foodDTO `json:"food"`
}

type dishDTO struct {
	ID       string       `json:"id"`
	NameES   string       `json:"nameEs"`
	NameEN   string       `json:"nameEn"`
	Exact    bool         `json:"exact"` // cierra dentro de ±5 % kcal y ±5 g de proteína
	Macros   macrosDTO    `json:"macros"`
	Portions []portionDTO `json:"portions"`
}

type closingDTO struct {
	MealsLeft int       `json:"mealsLeft"`
	Need      macrosDTO `json:"need"`
	NetCarbs  bool      `json:"netCarbs"`
	Closed    bool      `json:"closed"`
	Dishes    []dishDTO `json:"dishes"`
}

// list: qué comer ahora. No usa IA ni créditos.
func (s *suggestionHandlers) list(w http.ResponseWriter, r *http.Request) {
	v, err := s.uc.Suggestions(r.Context(), userID(r.Context()))
	if err != nil {
		fail(w, s.h.log, err)
		return
	}
	out := closingDTO{MealsLeft: v.MealsLeft, Need: toMacrosDTO(v.Need), NetCarbs: v.NetCarbs, Closed: v.Closed, Dishes: []dishDTO{}}
	for _, d := range v.Dishes {
		dto := dishDTO{ID: d.ID, NameES: d.NameES, NameEN: d.NameEN, Exact: d.Exact, Macros: toMacrosDTO(d.Macros)}
		for _, p := range d.Portions {
			dto.Portions = append(dto.Portions, portionDTO{Grams: p.Grams, Food: toFoodDTO(p.Food)})
		}
		out.Dishes = append(out.Dishes, dto)
	}
	writeJSON(w, http.StatusOK, out)
}
