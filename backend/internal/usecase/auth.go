// Package usecase implementa los casos de uso (puertos de entrada). Orquesta el
// dominio y los puertos de salida; no conoce HTTP, SQL ni proveedores concretos.
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

// DefaultRefreshTTL es la vida de un refresh token. Cada renovación emite uno
// nuevo, así que un usuario activo nunca vuelve a iniciar sesión.
const DefaultRefreshTTL = 60 * 24 * time.Hour

// AuthService gestiona cuentas y sesiones con JWT propio (ver CLAUDE.md).
// Implementa ports.AuthUseCase.
type AuthService struct {
	users      ports.UserRepository
	sessions   ports.SessionStore
	tokens     ports.AccessTokenIssuer
	verifiers  map[ports.IdentityProvider]ports.IdentityVerifier
	refreshTTL time.Duration
	now        func() time.Time
}

// NewAuthService recibe todas sus dependencias explícitamente. verifiers puede
// no tener Apple o Google si no están configurados.
func NewAuthService(
	users ports.UserRepository,
	sessions ports.SessionStore,
	tokens ports.AccessTokenIssuer,
	verifiers map[ports.IdentityProvider]ports.IdentityVerifier,
	refreshTTL time.Duration,
	now func() time.Time,
) *AuthService {
	return &AuthService{users: users, sessions: sessions, tokens: tokens, verifiers: verifiers, refreshTTL: refreshTTL, now: now}
}

// CreateAnonymous crea una cuenta sin datos personales. La app la usa al
// terminar el onboarding para guardar el perfil antes de pedir ningún registro.
func (s *AuthService) CreateAnonymous(ctx context.Context) (ports.Session, error) {
	userID, err := s.users.CreateUser(ctx)
	if err != nil {
		return ports.Session{}, fmt.Errorf("creando usuario anónimo: %w", err)
	}
	return s.openSession(ctx, userID)
}

// SignIn inicia sesión con Apple o Google. Si la identidad no existe, crea la
// cuenta. created indica si es una cuenta nueva.
func (s *AuthService) SignIn(ctx context.Context, provider ports.IdentityProvider, idToken, nonce string) (sess ports.Session, created bool, err error) {
	ext, err := s.verify(ctx, provider, idToken, nonce)
	if err != nil {
		return ports.Session{}, false, err
	}
	userID, err := s.users.FindByIdentity(ctx, ext.Provider, ext.Subject)
	switch {
	case errors.Is(err, ports.ErrNotFound):
		if userID, err = s.users.CreateUser(ctx); err != nil {
			return ports.Session{}, false, fmt.Errorf("creando usuario: %w", err)
		}
		if err := s.users.LinkIdentity(ctx, userID, ext); err != nil {
			return ports.Session{}, false, fmt.Errorf("vinculando identidad nueva: %w", err)
		}
		created = true
	case err != nil:
		return ports.Session{}, false, fmt.Errorf("buscando identidad: %w", err)
	}
	sess, err = s.openSession(ctx, userID)
	return sess, created, err
}

// Link vincula Apple o Google a la cuenta actual (típicamente anónima), sin
// perder sus datos. Si esa identidad ya es de otra cuenta, devuelve
// ErrIdentityInUse y la app ofrece entrar a esa cuenta con SignIn.
func (s *AuthService) Link(ctx context.Context, userID domain.UserID, provider ports.IdentityProvider, idToken, nonce string) error {
	ext, err := s.verify(ctx, provider, idToken, nonce)
	if err != nil {
		return err
	}
	if err := s.users.LinkIdentity(ctx, userID, ext); err != nil {
		return fmt.Errorf("vinculando %s: %w", provider, err)
	}
	return nil
}

// Refresh canjea un refresh token por una sesión nueva (rotación).
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (ports.Session, error) {
	oldHash, err := hashRefreshToken(refreshToken)
	if err != nil {
		return ports.Session{}, err
	}
	newToken, newHash, err := newRefreshToken()
	if err != nil {
		return ports.Session{}, err
	}
	now := s.now()
	refreshExp := now.Add(s.refreshTTL)
	userID, err := s.sessions.Rotate(ctx, oldHash, newHash, refreshExp, now)
	if err != nil {
		return ports.Session{}, fmt.Errorf("renovando sesión: %w", err)
	}
	access, accessExp, err := s.tokens.Issue(userID, now)
	if err != nil {
		return ports.Session{}, fmt.Errorf("emitiendo access token: %w", err)
	}
	return ports.Session{UserID: userID, AccessToken: access, AccessExpiresAt: accessExp, RefreshToken: newToken, RefreshExpiresAt: refreshExp}, nil
}

// Logout cierra la sesión del refresh token (toda su familia).
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	h, err := hashRefreshToken(refreshToken)
	if err != nil {
		return nil // un token mal formado no corresponde a ninguna sesión: nada que cerrar
	}
	if err := s.sessions.Revoke(ctx, h, s.now()); err != nil {
		return fmt.Errorf("cerrando sesión: %w", err)
	}
	return nil
}

// DeleteAccount borra la cuenta y todos sus datos (exigencia del App Store).
// Pendiente: revocar el token de Sign in with Apple (ver pendientes.md).
func (s *AuthService) DeleteAccount(ctx context.Context, userID domain.UserID) error {
	if err := s.users.DeleteUser(ctx, userID); err != nil {
		return fmt.Errorf("eliminando cuenta: %w", err)
	}
	return nil
}

func (s *AuthService) verify(ctx context.Context, provider ports.IdentityProvider, idToken, nonce string) (ports.ExternalIdentity, error) {
	v, ok := s.verifiers[provider]
	if !ok {
		return ports.ExternalIdentity{}, fmt.Errorf("%w: %s", ports.ErrProviderNotConfigured, provider)
	}
	ext, err := v.Verify(ctx, idToken, nonce)
	if err != nil {
		return ports.ExternalIdentity{}, fmt.Errorf("verificando token de %s: %w", provider, err)
	}
	return ext, nil
}

func (s *AuthService) openSession(ctx context.Context, userID domain.UserID) (ports.Session, error) {
	token, hash, err := newRefreshToken()
	if err != nil {
		return ports.Session{}, err
	}
	now := s.now()
	refreshExp := now.Add(s.refreshTTL)
	if err := s.sessions.Create(ctx, userID, hash, refreshExp); err != nil {
		return ports.Session{}, fmt.Errorf("guardando sesión: %w", err)
	}
	access, accessExp, err := s.tokens.Issue(userID, now)
	if err != nil {
		return ports.Session{}, fmt.Errorf("emitiendo access token: %w", err)
	}
	return ports.Session{UserID: userID, AccessToken: access, AccessExpiresAt: accessExp, RefreshToken: token, RefreshExpiresAt: refreshExp}, nil
}

// refreshTokenBytes: 256 bits aleatorios; imposible de adivinar.
const refreshTokenBytes = 32

func newRefreshToken() (token string, hash []byte, err error) {
	b := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("generando refresh token: %w", err)
	}
	sum := sha256.Sum256(b)
	return base64.RawURLEncoding.EncodeToString(b), sum[:], nil
}

func hashRefreshToken(token string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(b) != refreshTokenBytes {
		return nil, fmt.Errorf("%w: refresh token mal formado", ports.ErrInvalidCredentials)
	}
	sum := sha256.Sum256(b)
	return sum[:], nil
}

var _ ports.AuthUseCase = (*AuthService)(nil)
