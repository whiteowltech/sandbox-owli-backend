// Package oidc verifica los tokens de identidad de Sign in with Apple y Google
// contra las claves públicas (JWKS) de cada proveedor.
package oidc

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/whiteowltech/owli/backend/internal/ports"
)

// Config describe un proveedor OpenID Connect.
type Config struct {
	Provider  ports.IdentityProvider
	Issuers   []string // valores aceptados de "iss"
	Audiences []string // client IDs propios (bundle id de iOS, client id de Google…)
	JWKSURL   string
	// NonceHashed: Apple pone en el token el SHA-256 (hex) del nonce que mandó la
	// app; Google pone el nonce tal cual.
	NonceHashed bool
}

// Apple devuelve la configuración de Sign in with Apple.
func Apple(audiences []string) Config {
	return Config{
		Provider:    ports.ProviderApple,
		Issuers:     []string{"https://appleid.apple.com"},
		Audiences:   audiences,
		JWKSURL:     "https://appleid.apple.com/auth/keys",
		NonceHashed: true,
	}
}

// Google devuelve la configuración de Sign in with Google.
func Google(audiences []string) Config {
	return Config{
		Provider:  ports.ProviderGoogle,
		Issuers:   []string{"https://accounts.google.com", "accounts.google.com"},
		Audiences: audiences,
		JWKSURL:   "https://www.googleapis.com/oauth2/v3/certs",
	}
}

// Verifier implementa ports.IdentityVerifier.
type Verifier struct {
	cfg    Config
	client *http.Client
	now    func() time.Time

	mu          sync.Mutex
	keys        map[string]*rsa.PublicKey
	fetchedAt   time.Time
	lastAttempt time.Time
}

// keysTTL: los proveedores rotan sus claves cada tanto; se refrescan por hora
// o al ver un kid desconocido (como mucho una vez por minuto).
const (
	keysTTL         = time.Hour
	minRefetchDelay = time.Minute
)

func New(cfg Config, client *http.Client, now func() time.Time) (*Verifier, error) {
	if len(cfg.Audiences) == 0 {
		return nil, fmt.Errorf("%w: %s sin client IDs", ports.ErrProviderNotConfigured, cfg.Provider)
	}
	return &Verifier{cfg: cfg, client: client, now: now, keys: map[string]*rsa.PublicKey{}}, nil
}

type idClaims struct {
	jwt.RegisteredClaims
	Nonce         string `json:"nonce"`
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"` // Apple lo manda como "true"; Google como true
}

// Verify valida firma (RS256), emisor, audiencia, vencimiento y nonce.
func (v *Verifier) Verify(ctx context.Context, idToken, nonce string) (ports.ExternalIdentity, error) {
	claims := &idClaims{}
	_, err := jwt.ParseWithClaims(idToken, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return v.key(ctx, kid)
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(v.now),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		if errors.Is(err, ports.ErrDependencyUnavailable) {
			return ports.ExternalIdentity{}, err
		}
		return ports.ExternalIdentity{}, fmt.Errorf("%w: %v", ports.ErrInvalidCredentials, err)
	}
	if !slices.Contains(v.cfg.Issuers, claims.Issuer) {
		return ports.ExternalIdentity{}, fmt.Errorf("%w: emisor %q", ports.ErrInvalidCredentials, claims.Issuer)
	}
	if !slices.ContainsFunc(claims.Audience, func(a string) bool { return slices.Contains(v.cfg.Audiences, a) }) {
		return ports.ExternalIdentity{}, fmt.Errorf("%w: audiencia no reconocida", ports.ErrInvalidCredentials)
	}
	if claims.Subject == "" {
		return ports.ExternalIdentity{}, fmt.Errorf("%w: token sin sujeto", ports.ErrInvalidCredentials)
	}
	if nonce != "" && claims.Nonce != v.expectedNonce(nonce) {
		return ports.ExternalIdentity{}, fmt.Errorf("%w: nonce no coincide", ports.ErrInvalidCredentials)
	}
	ext := ports.ExternalIdentity{Provider: v.cfg.Provider, Subject: claims.Subject}
	if verified(claims.EmailVerified) {
		ext.Email = claims.Email
	}
	return ext, nil
}

func (v *Verifier) expectedNonce(raw string) string {
	if !v.cfg.NonceHashed {
		return raw
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func verified(x any) bool {
	switch t := x.(type) {
	case bool:
		return t
	case string:
		return t == "true"
	}
	return false
}

func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.now()
	stale := now.Sub(v.fetchedAt) > keysTTL
	if k, ok := v.keys[kid]; ok && !stale {
		return k, nil
	}
	if now.Sub(v.lastAttempt) >= minRefetchDelay || stale {
		v.lastAttempt = now
		keys, err := v.fetch(ctx)
		if err != nil {
			if k, ok := v.keys[kid]; ok {
				return k, nil // claves algo viejas pero conocidas: mejor que fallar
			}
			return nil, err
		}
		v.keys, v.fetchedAt = keys, now
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("clave %q desconocida", kid)
}

type jwks struct {
	Keys []struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func (v *Verifier) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.cfg.JWKSURL, nil)
	if err != nil {
		return nil, fmt.Errorf("armando pedido de claves: %w", err)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: claves de %s: %v", ports.ErrDependencyUnavailable, v.cfg.Provider, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: claves de %s respondieron %d", ports.ErrDependencyUnavailable, v.cfg.Provider, resp.StatusCode)
	}
	var set jwks
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return nil, fmt.Errorf("%w: claves de %s ilegibles: %v", ports.ErrDependencyUnavailable, v.cfg.Provider, err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		n, errN := base64.RawURLEncoding.DecodeString(k.N)
		e, errE := base64.RawURLEncoding.DecodeString(k.E)
		if errN != nil || errE != nil || len(e) == 0 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	return keys, nil
}
