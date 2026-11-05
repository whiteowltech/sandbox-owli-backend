package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

type diaryHandlers struct {
	diary ports.DiaryUseCase
	h     *handlers
}

type macrosDTO struct {
	Kcal     float64 `json:"kcal"`
	ProteinG float64 `json:"proteinG"`
	CarbsG   float64 `json:"carbsG"`
	FiberG   float64 `json:"fiberG"`
	FatG     float64 `json:"fatG"`
}

func toMacrosDTO(m domain.Macros) macrosDTO {
	return macrosDTO{Kcal: m.Calories, ProteinG: m.ProteinG, CarbsG: m.CarbsG, FiberG: m.FiberG, FatG: m.FatG}
}

// microsDTO: null = sin dato (nunca se manda un cero inventado).
type microsDTO struct {
	VitaminCMg   *float64 `json:"vitaminCMg"`
	CalciumMg    *float64 `json:"calciumMg"`
	IronMg       *float64 `json:"ironMg"`
	MagnesiumMg  *float64 `json:"magnesiumMg"`
	PotassiumMg  *float64 `json:"potassiumMg"`
	SodiumMg     *float64 `json:"sodiumMg"`
	ZincMg       *float64 `json:"zincMg"`
	VitaminB12Ug *float64 `json:"vitaminB12Ug"`
}

func toMicrosDTO(m domain.Micros) microsDTO {
	return microsDTO{VitaminCMg: m.VitaminCMg, CalciumMg: m.CalciumMg, IronMg: m.IronMg, MagnesiumMg: m.MagnesiumMg,
		PotassiumMg: m.PotassiumMg, SodiumMg: m.SodiumMg, ZincMg: m.ZincMg, VitaminB12Ug: m.VitaminB12Ug}
}

type traitsDTO struct {
	AnimalOrigin       bool `json:"animalOrigin"`
	PlantOrigin        bool `json:"plantOrigin"`
	ContainsMeat       bool `json:"containsMeat"`
	ContainsFish       bool `json:"containsFish"`
	ContainsGrain      bool `json:"containsGrain"`
	ContainsLegume     bool `json:"containsLegume"`
	ContainsDairy      bool `json:"containsDairy"`
	ContainsAddedSugar bool `json:"containsAddedSugar"`
	ContainsGluten     bool `json:"containsGluten"`
	ContainsLactose    bool `json:"containsLactose"`
	AllergensVerified  bool `json:"allergensVerified"`
}

type foodDTO struct {
	ID      string    `json:"id"`
	NameES  string    `json:"nameEs"`
	NameEN  string    `json:"nameEn"`
	Per100g macrosDTO `json:"per100g"`
	Micros  microsDTO `json:"micros"`
	Traits  traitsDTO `json:"traits"`
}

func toFoodDTO(f domain.Food) foodDTO {
	t := f.Traits
	return foodDTO{ID: string(f.ID), NameES: f.NameES, NameEN: f.NameEN, Per100g: toMacrosDTO(f.Per100g), Micros: toMicrosDTO(f.Micros),
		Traits: traitsDTO{AnimalOrigin: t.AnimalOrigin, PlantOrigin: t.PlantOrigin, ContainsMeat: t.ContainsMeat, ContainsFish: t.ContainsFish,
			ContainsGrain: t.ContainsGrain, ContainsLegume: t.ContainsLegume, ContainsDairy: t.ContainsDairy,
			ContainsAddedSugar: t.ContainsAddedSugar, ContainsGluten: t.ContainsGluten, ContainsLactose: t.ContainsLactose,
			AllergensVerified: t.AllergensVerified}}
}

type entryDTO struct {
	ID      string    `json:"id"`
	FoodID  string    `json:"foodId"`
	NameES  string    `json:"nameEs"`
	NameEN  string    `json:"nameEn"`
	Grams   float64   `json:"grams"`
	EatenAt time.Time `json:"eatenAt"`
	Source  string    `json:"source"`
	Macros  macrosDTO `json:"macros"`
}

func toEntryDTO(e ports.LoggedEntry) entryDTO {
	return entryDTO{ID: e.ID, FoodID: string(e.FoodID), NameES: e.FoodNameES, NameEN: e.FoodNameEN, Grams: e.Grams,
		EatenAt: e.EatenAt, Source: string(e.Source), Macros: toMacrosDTO(e.Macros)}
}

type microTotalDTO struct {
	Value *float64 `json:"value"` // suma de los alimentos con dato
	Known int      `json:"known"` // cuántos alimentos tenían el dato
}

type dayDTO struct {
	Date    string                   `json:"date"`
	Entries []entryDTO               `json:"entries"`
	Totals  macrosDTO                `json:"totals"`
	Micros  map[string]microTotalDTO `json:"micros"`
	Items   int                      `json:"items"`
	Plan    planDTO                  `json:"plan"`
	Targets targetsDTO               `json:"targets"`
	Mood    string                   `json:"mood"`
}

type targetsDTO struct {
	ProteinG float64 `json:"proteinG"`
	CarbsG   float64 `json:"carbsG"`
	FatG     float64 `json:"fatG"`
	NetCarbs bool    `json:"netCarbs"`
}

func toDayDTO(d ports.DayView) dayDTO {
	entries := make([]entryDTO, len(d.Entries))
	for i, e := range d.Entries {
		entries[i] = toEntryDTO(e)
	}
	s := d.Micros.Sum
	values := []*float64{s.VitaminCMg, s.CalciumMg, s.IronMg, s.MagnesiumMg, s.PotassiumMg, s.SodiumMg, s.ZincMg, s.VitaminB12Ug}
	keys := []string{"vitaminCMg", "calciumMg", "ironMg", "magnesiumMg", "potassiumMg", "sodiumMg", "zincMg", "vitaminB12Ug"}
	micros := map[string]microTotalDTO{}
	for i, k := range keys {
		micros[k] = microTotalDTO{Value: values[i], Known: d.Micros.Known[i]}
	}
	t := d.Targets
	return dayDTO{Date: d.Date, Entries: entries, Totals: toMacrosDTO(d.Totals), Micros: micros, Items: d.Micros.Items,
		Plan: toPlanDTO(d.Plan), Targets: targetsDTO{ProteinG: t.ProteinG, CarbsG: t.CarbsG, FatG: t.FatG, NetCarbs: t.NetCarbs},
		Mood: string(d.Mood)}
}

func (d *diaryHandlers) searchFoods(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 20
	}
	foods, err := d.diary.SearchFoods(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		fail(w, d.h.log, err)
		return
	}
	out := make([]foodDTO, len(foods))
	for i, f := range foods {
		out[i] = toFoodDTO(f)
	}
	writeJSON(w, http.StatusOK, map[string]any{"foods": out})
}

type logRequest struct {
	FoodID  string     `json:"foodId"`
	Grams   float64    `json:"grams"`
	EatenAt *time.Time `json:"eatenAt"` // opcional: por defecto, ahora
	Source  string     `json:"source"`  // opcional: manual (por defecto), photo, voice, suggestion
}

func (d *diaryHandlers) logEntry(w http.ResponseWriter, r *http.Request) {
	var req logRequest
	if !decode(w, r, &req) {
		return
	}
	var at time.Time
	if req.EatenAt != nil {
		at = *req.EatenAt
	}
	src := domain.SourceManual
	if req.Source != "" {
		src = domain.EntrySource(req.Source) // el dominio rechaza orígenes desconocidos
	}
	e, err := d.diary.Log(r.Context(), userID(r.Context()), domain.FoodID(req.FoodID), req.Grams, at, src)
	if err != nil {
		fail(w, d.h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, toEntryDTO(e))
}

func (d *diaryHandlers) deleteEntry(w http.ResponseWriter, r *http.Request) {
	if err := d.diary.Delete(r.Context(), userID(r.Context()), chi.URLParam(r, "id")); err != nil {
		fail(w, d.h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d *diaryHandlers) day(w http.ResponseWriter, r *http.Request) {
	view, err := d.diary.Day(r.Context(), userID(r.Context()), r.URL.Query().Get("date"))
	if err != nil {
		fail(w, d.h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, toDayDTO(view))
}

type weekDayDTO struct {
	Date       string    `json:"date"`
	Totals     macrosDTO `json:"totals"`
	Entries    int       `json:"entries"`
	InProgress bool      `json:"inProgress"`
	Mood       string    `json:"mood"`
	Closed     bool      `json:"closed"`
}

type weekDTO struct {
	Start  string       `json:"start"`
	End    string       `json:"end"`
	Today  string       `json:"today"`
	Days   []weekDayDTO `json:"days"`
	Target struct {
		Kcal     int     `json:"kcal"`
		ProteinG float64 `json:"proteinG"`
	} `json:"target"`
	Review struct {
		LoggedDays       int      `json:"loggedDays"`
		ClosedDays       int      `json:"closedDays"`
		ProteinShortDays int      `json:"proteinShortDays"`
		OverDays         int      `json:"overDays"`
		AvgKcal          *float64 `json:"avgKcal"` // null si no hay días anotados
		AvgProteinG      *float64 `json:"avgProteinG"`
		Streak           int      `json:"streak"`
	} `json:"review"`
}

// week: los 7 días que terminan en ?end=AAAA-MM-DD (por defecto, hoy).
func (d *diaryHandlers) week(w http.ResponseWriter, r *http.Request) {
	v, err := d.diary.Week(r.Context(), userID(r.Context()), r.URL.Query().Get("end"))
	if err != nil {
		fail(w, d.h.log, err)
		return
	}
	out := weekDTO{Start: v.Start, End: v.End, Today: v.Today, Days: make([]weekDayDTO, len(v.Days))}
	for i, day := range v.Days {
		out.Days[i] = weekDayDTO{Date: day.Date, Totals: toMacrosDTO(day.Totals), Entries: day.Entries,
			InProgress: day.InProgress, Mood: string(day.Mood), Closed: day.Closed()}
	}
	out.Target.Kcal, out.Target.ProteinG = v.Plan.DailyTarget, v.Targets.ProteinG
	rv := v.Review
	out.Review.LoggedDays, out.Review.ClosedDays, out.Review.Streak = rv.LoggedDays, rv.ClosedDays, rv.Streak
	out.Review.ProteinShortDays, out.Review.OverDays = rv.ProteinShortDays, rv.OverDays
	if rv.HasAverages {
		out.Review.AvgKcal, out.Review.AvgProteinG = &rv.AvgKcal, &rv.AvgProteinG
	}
	writeJSON(w, http.StatusOK, out)
}
