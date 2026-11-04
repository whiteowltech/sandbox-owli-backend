package ports

import (
	"context"
	"time"

	"github.com/whiteowltech/owli/backend/internal/domain"
)

// Session es lo que recibe la app al iniciar o renovar una sesión.
type Session struct {
	UserID           domain.UserID
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// AuthUseCase: cuentas y sesiones (implementado en usecase.AuthService).
type AuthUseCase interface {
	CreateAnonymous(ctx context.Context) (Session, error)
	SignIn(ctx context.Context, provider IdentityProvider, idToken, nonce string) (s Session, created bool, err error)
	Link(ctx context.Context, userID domain.UserID, provider IdentityProvider, idToken, nonce string) error
	Refresh(ctx context.Context, refreshToken string) (Session, error)
	Logout(ctx context.Context, refreshToken string) error
	DeleteAccount(ctx context.Context, userID domain.UserID) error
}

// ProfileUseCase: perfil del usuario autenticado (implementado en usecase.ProfileService).
type ProfileUseCase interface {
	Get(ctx context.Context, userID domain.UserID) (domain.UserProfile, error)
	Save(ctx context.Context, userID domain.UserID, p domain.UserProfile) (domain.Plan, error)
}
