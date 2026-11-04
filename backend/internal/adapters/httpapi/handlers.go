package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

type handlers struct {
	auth     ports.AuthUseCase
	profiles ports.ProfileUseCase
	log      *slog.Logger
}

// maxBody: ningún pedido de esta API necesita más (contra cuerpos gigantes).
const maxBody = 16 << 10

// decode lee JSON estricto: campos desconocidos o cuerpos extra son error.
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, nil, apiError{http.StatusBadRequest, "bad_request", "El cuerpo del pedido no es JSON válido para esta operación."})
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		writeError(w, nil, apiError{http.StatusBadRequest, "bad_request", "El cuerpo tiene datos de más."})
		return false
	}
	return true
}

// ---- Auth ----

type sessionDTO struct {
	UserID           string    `json:"userId"`
	AccessToken      string    `json:"accessToken"`
	AccessExpiresAt  time.Time `json:"accessExpiresAt"`
	RefreshToken     string    `json:"refreshToken"`
	RefreshExpiresAt time.Time `json:"refreshExpiresAt"`
	Created          *bool     `json:"created,omitempty"`
}

func toSessionDTO(s ports.Session) sessionDTO {
	return sessionDTO{UserID: string(s.UserID), AccessToken: s.AccessToken, AccessExpiresAt: s.AccessExpiresAt,
		RefreshToken: s.RefreshToken, RefreshExpiresAt: s.RefreshExpiresAt}
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type idTokenRequest struct {
	IDToken string `json:"idToken"`
	Nonce   string `json:"nonce"`
}

func provider(r *http.Request) ports.IdentityProvider {
	return ports.IdentityProvider(chi.URLParam(r, "provider"))
}

func (h *handlers) createAnonymous(w http.ResponseWriter, r *http.Request) {
	s, err := h.auth.CreateAnonymous(r.Context())
	if err != nil {
		fail(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, toSessionDTO(s))
}

func (h *handlers) signIn(w http.ResponseWriter, r *http.Request) {
	var req idTokenRequest
	if !decode(w, r, &req) {
		return
	}
	s, created, err := h.auth.SignIn(r.Context(), provider(r), req.IDToken, req.Nonce)
	if err != nil {
		fail(w, h.log, err)
		return
	}
	dto := toSessionDTO(s)
	dto.Created = &created
	writeJSON(w, http.StatusOK, dto)
}

func (h *handlers) link(w http.ResponseWriter, r *http.Request) {
	var req idTokenRequest
	if !decode(w, r, &req) {
		return
	}
	if err := h.auth.Link(r.Context(), userID(r.Context()), provider(r), req.IDToken, req.Nonce); err != nil {
		fail(w, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !decode(w, r, &req) {
		return
	}
	s, err := h.auth.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		fail(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, toSessionDTO(s))
}

func (h *handlers) logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !decode(w, r, &req) {
		return
	}
	if err := h.auth.Logout(r.Context(), req.RefreshToken); err != nil {
		fail(w, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) deleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := h.auth.DeleteAccount(r.Context(), userID(r.Context())); err != nil {
		fail(w, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Perfil ----

// profileDTO usa los mismos nombres que el modelo de la app (app/src/app/core/profile/profile.model.ts).
type profileDTO struct {
	Name           string   `json:"name"`
	Sex            string   `json:"sex"`
	AgeYears       int      `json:"ageYears"`
	HeightCm       float64  `json:"heightCm"`
	WeightKg       float64  `json:"weightKg"`
	Activity       string   `json:"activity"`
	Goal           string   `json:"goal"`
	Pace           string   `json:"pace"`
	TargetWeightKg float64  `json:"targetWeightKg"`
	Diet           string   `json:"diet"`
	Restrictions   []string `json:"restrictions"`
	MealsPerDay    int      `json:"mealsPerDay"`
	Units          string   `json:"units"`
	Timezone       string   `json:"timezone"`
}

type planDTO struct {
	TDEE                  int      `json:"tdee"`
	DailyTarget           int      `json:"dailyTarget"`
	MinSafe               int      `json:"minSafe"`
	Clamped               bool     `json:"clamped"`
	WeeklyRateKg          float64  `json:"weeklyRateKg"`
	WeeksToGoal           *float64 `json:"weeksToGoal"`
	WrongDirection        bool     `json:"wrongDirection"`
	TooSlow               bool     `json:"tooSlow"`
	TargetBelowHealthyBMI bool     `json:"targetBelowHealthyBmi"`
}

type profileResponse struct {
	Profile profileDTO `json:"profile"`
	Plan    planDTO    `json:"plan"`
}

func (d profileDTO) toDomain() domain.UserProfile {
	restr := make([]domain.Restriction, len(d.Restrictions))
	for i, x := range d.Restrictions {
		restr[i] = domain.Restriction(x)
	}
	return domain.UserProfile{
		Name: d.Name,
		Bio: domain.Biometrics{WeightKg: d.WeightKg, HeightCm: d.HeightCm, AgeYears: d.AgeYears,
			Sex: domain.Sex(d.Sex), Activity: domain.ActivityLevel(d.Activity)},
		Goal: domain.Goal(d.Goal), Pace: domain.Pace(d.Pace), TargetWeightKg: d.TargetWeightKg,
		Diet: domain.DietType(d.Diet), Restrictions: restr, MealsPerDay: d.MealsPerDay,
		Units: domain.UnitSystem(d.Units), Timezone: d.Timezone,
	}
}

func fromDomain(p domain.UserProfile) profileDTO {
	restr := make([]string, len(p.Restrictions))
	for i, x := range p.Restrictions {
		restr[i] = string(x)
	}
	return profileDTO{
		Name: p.Name, Sex: string(p.Bio.Sex), AgeYears: p.Bio.AgeYears, HeightCm: p.Bio.HeightCm, WeightKg: p.Bio.WeightKg,
		Activity: string(p.Bio.Activity), Goal: string(p.Goal), Pace: string(p.Pace), TargetWeightKg: p.TargetWeightKg,
		Diet: string(p.Diet), Restrictions: restr, MealsPerDay: p.MealsPerDay, Units: string(p.Units), Timezone: p.Timezone,
	}
}

func toPlanDTO(p domain.Plan) planDTO {
	return planDTO{TDEE: p.TDEE, DailyTarget: p.DailyTarget, MinSafe: p.MinSafe, Clamped: p.Clamped,
		WeeklyRateKg: p.WeeklyRateKg, WeeksToGoal: p.WeeksToGoal, WrongDirection: p.WrongDirection,
		TooSlow: p.TooSlow, TargetBelowHealthyBMI: p.TargetBelowHealthyBMI}
}

func (h *handlers) getProfile(w http.ResponseWriter, r *http.Request) {
	p, err := h.profiles.Get(r.Context(), userID(r.Context()))
	if err != nil {
		fail(w, h.log, err)
		return
	}
	plan, err := domain.DefaultGoalPolicy.PlanFor(domain.PlanInput{Bio: p.Bio, Goal: p.Goal, Pace: p.Pace, TargetWeightKg: p.TargetWeightKg})
	if err != nil {
		fail(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, profileResponse{Profile: fromDomain(p), Plan: toPlanDTO(plan)})
}

func (h *handlers) putProfile(w http.ResponseWriter, r *http.Request) {
	var dto profileDTO
	if !decode(w, r, &dto) {
		return
	}
	p := dto.toDomain()
	plan, err := h.profiles.Save(r.Context(), userID(r.Context()), p)
	if err != nil {
		fail(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, profileResponse{Profile: fromDomain(p), Plan: toPlanDTO(plan)})
}
