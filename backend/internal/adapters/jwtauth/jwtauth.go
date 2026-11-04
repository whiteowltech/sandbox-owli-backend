// Package jwtauth emite y verifica los access tokens propios de Owli (JWT HS256).
//
// Rotación del secreto sin cortar sesiones: cada token lleva en `kid` un
// identificador derivado del secreto que lo firmó. Para rotar, el secreto nuevo
// pasa a JWT_SECRET y el anterior a JWT_SECRET_PREVIOUS; los tokens viejos siguen
// siendo válidos hasta vencer (15 min) y después se quita el anterior.
package jwtauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

const (
	// MinSecretBytes: un secreto HS256 más corto es adivinable por fuerza bruta.
	MinSecretBytes = 32
	// DefaultTTL: vida corta; la sesión larga la sostiene el refresh token.
	DefaultTTL = 15 * time.Minute

	issuer   = "owli-api"
	audience = "owli-app"
)

// ErrWeakSecret indica un secreto ausente o demasiado corto.
var ErrWeakSecret = errors.New("JWT_SECRET debe tener al menos 32 bytes")

// Manager implementa ports.AccessTokenIssuer y ports.AuthVerifier.
type Manager struct {
	activeKID string
	keys      map[string][]byte // kid → secreto
	ttl       time.Duration
	now       func() time.Time
}

// New recibe el secreto activo y, opcionalmente, el anterior (rotación).
func New(active, previous string, ttl time.Duration, now func() time.Time) (*Manager, error) {
	if len(active) < MinSecretBytes {
		return nil, ErrWeakSecret
	}
	m := &Manager{keys: map[string][]byte{}, ttl: ttl, now: now}
	m.activeKID = keyID(active)
	m.keys[m.activeKID] = []byte(active)
	if previous != "" {
		if len(previous) < MinSecretBytes {
			return nil, fmt.Errorf("JWT_SECRET_PREVIOUS: %w", ErrWeakSecret)
		}
		m.keys[keyID(previous)] = []byte(previous)
	}
	return m, nil
}

// keyID deriva un identificador público del secreto sin revelarlo.
func keyID(secret string) string {
	sum := sha256.Sum256([]byte("owli-kid:" + secret))
	return hex.EncodeToString(sum[:4])
}

// Issue firma un access token para el usuario.
func (m *Manager) Issue(userID domain.UserID, now time.Time) (string, time.Time, error) {
	exp := now.Add(m.ttl)
	claims := jwt.RegisteredClaims{
		Subject:   string(userID),
		Issuer:    issuer,
		Audience:  jwt.ClaimStrings{audience},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok.Header["kid"] = m.activeKID
	signed, err := tok.SignedString(m.keys[m.activeKID])
	if err != nil {
		return "", time.Time{}, fmt.Errorf("firmando access token: %w", err)
	}
	return signed, exp, nil
}

// Verify valida firma, algoritmo, emisor, audiencia y vencimiento.
func (m *Manager) Verify(_ context.Context, token string) (domain.UserID, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		key, ok := m.keys[kid]
		if !ok {
			return nil, errors.New("kid desconocido")
		}
		return key, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), // nunca "none" ni otros algoritmos
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ports.ErrInvalidCredentials, err)
	}
	if claims.Subject == "" {
		return "", fmt.Errorf("%w: token sin sujeto", ports.ErrInvalidCredentials)
	}
	return domain.UserID(claims.Subject), nil
}
