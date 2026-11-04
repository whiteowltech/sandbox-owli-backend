package usecase

import (
	"context"
	"fmt"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

// ProfileService lee y guarda el perfil del usuario autenticado.
type ProfileService struct {
	profiles ports.ProfileRepository
}

func NewProfileService(profiles ports.ProfileRepository) *ProfileService {
	return &ProfileService{profiles: profiles}
}

// Get devuelve ports.ErrNotFound si el usuario aún no completó el onboarding.
func (s *ProfileService) Get(ctx context.Context, userID domain.UserID) (domain.UserProfile, error) {
	p, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("leyendo perfil: %w", err)
	}
	return p, nil
}

// Save valida el perfil con las reglas del dominio y lo guarda. También
// devuelve el plan calculado para que la app muestre el mismo objetivo.
func (s *ProfileService) Save(ctx context.Context, userID domain.UserID, p domain.UserProfile) (domain.Plan, error) {
	if err := p.Validate(); err != nil {
		return domain.Plan{}, err
	}
	plan, err := domain.DefaultGoalPolicy.PlanFor(domain.PlanInput{Bio: p.Bio, Goal: p.Goal, Pace: p.Pace, TargetWeightKg: p.TargetWeightKg})
	if err != nil {
		return domain.Plan{}, err
	}
	if p.Restrictions == nil {
		p.Restrictions = []domain.Restriction{}
	}
	if err := s.profiles.Upsert(ctx, userID, p); err != nil {
		return domain.Plan{}, fmt.Errorf("guardando perfil: %w", err)
	}
	return plan, nil
}

var _ ports.ProfileUseCase = (*ProfileService)(nil)
