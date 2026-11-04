package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/whiteowltech/owli/backend/internal/ports"
)

var t0 = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

type fakeProvider struct {
	key     *rsa.PrivateKey
	kid     string
	srv     *httptest.Server
	fetches atomic.Int32
}

func newFakeProvider(t *testing.T) *fakeProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &fakeProvider{key: key, kid: "k1"}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		p.fetches.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": p.kid, "kty": "RSA", "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *fakeProvider) sign(t *testing.T, claims jwt.MapClaims, kid string, key *rsa.PrivateKey) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func appleVerifier(t *testing.T, p *fakeProvider, now *time.Time) *Verifier {
	t.Helper()
	cfg := Apple([]string{"org.whiteowltech.owli"})
	cfg.JWKSURL = p.srv.URL
	v, err := New(cfg, p.srv.Client(), func() time.Time { return *now })
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func appleClaims(nonce string) jwt.MapClaims {
	sum := sha256.Sum256([]byte(nonce))
	return jwt.MapClaims{
		"iss": "https://appleid.apple.com", "aud": "org.whiteowltech.owli", "sub": "001234.apple",
		"exp": t0.Add(10 * time.Minute).Unix(), "iat": t0.Unix(),
		"nonce": hex.EncodeToString(sum[:]), "email": "x@privaterelay.appleid.com", "email_verified": "true",
	}
}

func TestAppleTokenValid(t *testing.T) {
	p := newFakeProvider(t)
	now := t0
	v := appleVerifier(t, p, &now)
	got, err := v.Verify(context.Background(), p.sign(t, appleClaims("n-1"), p.kid, p.key), "n-1")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Provider != ports.ProviderApple || got.Subject != "001234.apple" || got.Email != "x@privaterelay.appleid.com" {
		t.Errorf("identidad = %+v", got)
	}
}

func TestAppleTokenRejected(t *testing.T) {
	p := newFakeProvider(t)
	now := t0
	v := appleVerifier(t, p, &now)
	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	mutate := func(f func(jwt.MapClaims)) jwt.MapClaims { c := appleClaims("n-1"); f(c); return c }
	tests := map[string]string{
		"firmado por otra clave": p.sign(t, appleClaims("n-1"), p.kid, otherKey),
		"otra audiencia":         p.sign(t, mutate(func(c jwt.MapClaims) { c["aud"] = "com.otra.app" }), p.kid, p.key),
		"otro emisor":            p.sign(t, mutate(func(c jwt.MapClaims) { c["iss"] = "https://evil.example" }), p.kid, p.key),
		"vencido":                p.sign(t, mutate(func(c jwt.MapClaims) { c["exp"] = t0.Add(-time.Hour).Unix() }), p.kid, p.key),
		"sin vencimiento":        p.sign(t, mutate(func(c jwt.MapClaims) { delete(c, "exp") }), p.kid, p.key),
		"nonce de otro pedido":   p.sign(t, appleClaims("n-OTRO"), p.kid, p.key),
		"sin sujeto":             p.sign(t, mutate(func(c jwt.MapClaims) { c["sub"] = "" }), p.kid, p.key),
		"kid desconocido":        p.sign(t, appleClaims("n-1"), "k-falso", p.key),
		"no es un token":         "basura",
	}
	for name, tok := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := v.Verify(context.Background(), tok, "n-1"); !errors.Is(err, ports.ErrInvalidCredentials) {
				t.Errorf("Verify = %v, se esperaba ErrInvalidCredentials", err)
			}
		})
	}
}

func TestGoogleNonceIsPlainAndEmailNeedsVerification(t *testing.T) {
	p := newFakeProvider(t)
	now := t0
	cfg := Google([]string{"client-123.apps.googleusercontent.com"})
	cfg.JWKSURL = p.srv.URL
	v, _ := New(cfg, p.srv.Client(), func() time.Time { return now })
	claims := jwt.MapClaims{
		"iss": "accounts.google.com", "aud": "client-123.apps.googleusercontent.com", "sub": "g-42",
		"exp": t0.Add(time.Hour).Unix(), "nonce": "plano", "email": "a@gmail.com", "email_verified": false,
	}
	got, err := v.Verify(context.Background(), p.sign(t, claims, p.kid, p.key), "plano")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Email != "" {
		t.Error("un correo no verificado no se guarda")
	}
}

func TestKeysAreCachedAndRefreshedOnRotation(t *testing.T) {
	p := newFakeProvider(t)
	now := t0
	v := appleVerifier(t, p, &now)
	tok := p.sign(t, appleClaims("n"), p.kid, p.key)
	for range 3 {
		if _, err := v.Verify(context.Background(), tok, "n"); err != nil {
			t.Fatal(err)
		}
	}
	if p.fetches.Load() != 1 {
		t.Errorf("las claves se pidieron %d veces, se esperaba 1 (caché)", p.fetches.Load())
	}
	// El proveedor rota su clave: un kid nuevo dispara un nuevo pedido.
	p.kid = "k2"
	now = t0.Add(2 * time.Minute)
	if _, err := v.Verify(context.Background(), p.sign(t, appleClaims("n"), "k2", p.key), "n"); err != nil {
		t.Fatalf("tras la rotación: %v", err)
	}
	if p.fetches.Load() != 2 {
		t.Errorf("pedidos = %d, se esperaba 2", p.fetches.Load())
	}
}

func TestProviderDownIsNotBlamedOnTheUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	cfg := Apple([]string{"org.whiteowltech.owli"})
	cfg.JWKSURL = srv.URL
	v, _ := New(cfg, srv.Client(), func() time.Time { return t0 })
	p := newFakeProvider(t)
	_, err := v.Verify(context.Background(), p.sign(t, appleClaims("n"), p.kid, p.key), "n")
	if !errors.Is(err, ports.ErrDependencyUnavailable) {
		t.Errorf("Apple caído = %v, se esperaba ErrDependencyUnavailable (503, no 401)", err)
	}
}

func TestRequiresAudiences(t *testing.T) {
	if _, err := New(Apple(nil), http.DefaultClient, time.Now); !errors.Is(err, ports.ErrProviderNotConfigured) {
		t.Errorf("sin client IDs = %v", err)
	}
}
