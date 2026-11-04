package ports

import (
	"context"
	"errors"
	"time"

	"github.com/whiteowltech/owli/backend/internal/domain"
)

// Errores de autenticación y de almacenamiento. El adapter HTTP los traduce a
// códigos de estado; los demás adapters los envuelven con %w.
var (
	ErrInvalidCredentials    = errors.New("credenciales inválidas o vencidas")
	ErrProviderNotConfigured = errors.New("proveedor de identidad no configurado")
	ErrSessionNotFound       = errors.New("sesión inexistente")
	ErrSessionExpired        = errors.New("sesión vencida")
	ErrSessionReused         = errors.New("refresh token reutilizado: se cerraron las sesiones de esa familia")
	ErrIdentityInUse         = errors.New("la identidad ya pertenece a otra cuenta")
	ErrNotFound              = errors.New("no encontrado")
	ErrRateLimited           = errors.New("demasiadas solicitudes")
	ErrAIBudgetExceeded      = errors.New("tope diario de gasto en IA alcanzado")
	ErrDependencyUnavailable = errors.New("dependencia no disponible")
)

// IdentityProvider es un proveedor externo de identidad.
type IdentityProvider string

const (
	ProviderApple  IdentityProvider = "apple"
	ProviderGoogle IdentityProvider = "google"
	ProviderEmail  IdentityProvider = "email"
)

// ExternalIdentity es la identidad ya verificada por el proveedor.
type ExternalIdentity struct {
	Provider IdentityProvider
	Subject  string // identificador estable del usuario en ese proveedor
	Email    string // vacío si el proveedor no lo da o no está verificado
}

// IdentityVerifier valida el token de identidad emitido por Apple o Google.
// Devuelve ErrInvalidCredentials si la firma, el emisor, la audiencia, el
// vencimiento o el nonce no son válidos.
type IdentityVerifier interface {
	Verify(ctx context.Context, idToken, nonce string) (ExternalIdentity, error)
}

// AuthVerifier resuelve un access token a un UserID (CLAUDE.md: es el único
// punto donde se confía en la identidad del llamador).
type AuthVerifier interface {
	Verify(ctx context.Context, accessToken string) (domain.UserID, error)
}

// AccessTokenIssuer emite access tokens de vida corta.
type AccessTokenIssuer interface {
	Issue(userID domain.UserID, now time.Time) (token string, expiresAt time.Time, err error)
}

// UserRepository guarda usuarios e identidades.
type UserRepository interface {
	CreateUser(ctx context.Context) (domain.UserID, error)
	// FindByIdentity devuelve ErrNotFound si nadie tiene esa identidad.
	FindByIdentity(ctx context.Context, provider IdentityProvider, subject string) (domain.UserID, error)
	// LinkIdentity asocia la identidad al usuario. Es idempotente si ya era suya;
	// devuelve ErrIdentityInUse si pertenece a otro usuario.
	LinkIdentity(ctx context.Context, userID domain.UserID, id ExternalIdentity) error
	// DeleteUser borra el usuario y, en cascada, todos sus datos.
	DeleteUser(ctx context.Context, userID domain.UserID) error
}

// SessionStore guarda los refresh tokens (solo su hash SHA-256).
type SessionStore interface {
	// Create abre una familia de sesión nueva.
	Create(ctx context.Context, userID domain.UserID, tokenHash []byte, expiresAt time.Time) error
	// Rotate canjea un refresh token por otro de la misma familia, de forma
	// atómica. Reglas:
	//   - token desconocido → ErrSessionNotFound
	//   - token ya usado o revocado → revoca toda la familia y ErrSessionReused
	//     (alguien más tiene una copia: posible robo)
	//   - token vencido → ErrSessionExpired
	Rotate(ctx context.Context, oldHash, newHash []byte, newExpiresAt, now time.Time) (domain.UserID, error)
	// Revoke cierra la familia del token (logout). No falla si no existe.
	Revoke(ctx context.Context, tokenHash []byte, now time.Time) error
}

// ProfileRepository guarda el perfil del usuario.
type ProfileRepository interface {
	// Get devuelve ErrNotFound si el usuario todavía no tiene perfil.
	Get(ctx context.Context, userID domain.UserID) (domain.UserProfile, error)
	Upsert(ctx context.Context, userID domain.UserID, p domain.UserProfile) error
}

// RateLimiter limita solicitudes por clave (IP, usuario, dispositivo) en una ventana.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration, err error)
}

// AISpendGuard es el tope global de gasto diario en IA (red de seguridad contra
// bots y errores). Se reserva antes de llamar al proveedor y se libera si falla.
type AISpendGuard interface {
	// Reserve devuelve ErrAIBudgetExceeded si el gasto del día superaría el tope.
	Reserve(ctx context.Context, microUSD int64, now time.Time) error
	Release(ctx context.Context, microUSD int64, now time.Time) error
}
