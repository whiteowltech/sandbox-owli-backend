package jwtauth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/whiteowltech/owli/backend/internal/ports"
)

const (
	secretA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	secretB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

var t0 = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

func manager(t *testing.T, active, previous string, now *time.Time) *Manager {
	t.Helper()
	m, err := New(active, previous, DefaultTTL, func() time.Time { return *now })
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m
}

func TestIssueAndVerify(t *testing.T) {
	now := t0
	m := manager(t, secretA, "", &now)
	tok, exp, err := m.Issue("user-1", now)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !exp.Equal(t0.Add(DefaultTTL)) {
		t.Errorf("vence %v, se esperaba %v", exp, t0.Add(DefaultTTL))
	}
	got, err := m.Verify(context.Background(), tok)
	if err != nil || got != "user-1" {
		t.Fatalf("Verify = %q, %v", got, err)
	}
}

func TestVerifyRejects(t *testing.T) {
	now := t0
	m := manager(t, secretA, "", &now)
	tok, _, _ := m.Issue("user-1", now)
	other := manager(t, secretB, "", &now)
	otherTok, _, _ := other.Issue("user-1", now)

	// Token sin firma (alg "none").
	none := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{Subject: "user-1", Issuer: issuer,
		Audience: jwt.ClaimStrings{audience}, ExpiresAt: jwt.NewNumericDate(t0.Add(time.Hour))})
	noneTok, _ := none.SignedString(jwt.UnsafeAllowNoneSignatureType)

	// Firmado con el secreto correcto pero para otra audiencia.
	wrongAud := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: "user-1", Issuer: issuer,
		Audience: jwt.ClaimStrings{"otra-app"}, ExpiresAt: jwt.NewNumericDate(t0.Add(time.Hour))})
	wrongAud.Header["kid"] = keyID(secretA)
	wrongAudTok, _ := wrongAud.SignedString([]byte(secretA))

	parts := strings.Split(tok, ".")
	tampered := parts[0] + "." + parts[1] + "x." + parts[2]

	tests := map[string]string{
		"firmado con otro secreto": otherTok,
		"alg none":                 noneTok,
		"otra audiencia":           wrongAudTok,
		"payload alterado":         tampered,
		"basura":                   "no.es.jwt",
		"vacío":                    "",
	}
	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := m.Verify(context.Background(), token); !errors.Is(err, ports.ErrInvalidCredentials) {
				t.Errorf("Verify = %v, se esperaba ErrInvalidCredentials", err)
			}
		})
	}

	t.Run("vencido", func(t *testing.T) {
		now = t0.Add(DefaultTTL + time.Second)
		defer func() { now = t0 }()
		if _, err := m.Verify(context.Background(), tok); !errors.Is(err, ports.ErrInvalidCredentials) {
			t.Errorf("token vencido = %v", err)
		}
	})
}

func TestRotationKeepsOldTokensValid(t *testing.T) {
	now := t0
	old := manager(t, secretA, "", &now)
	oldTok, _, _ := old.Issue("user-1", now)

	rotated := manager(t, secretB, secretA, &now)
	if got, err := rotated.Verify(context.Background(), oldTok); err != nil || got != "user-1" {
		t.Fatalf("durante la rotación el token viejo debe valer: %v", err)
	}
	newTok, _, _ := rotated.Issue("user-1", now)
	if _, err := old.Verify(context.Background(), newTok); err == nil {
		t.Error("un servidor sin el secreto nuevo no debe aceptar tokens nuevos")
	}

	done := manager(t, secretB, "", &now)
	if _, err := done.Verify(context.Background(), oldTok); err == nil {
		t.Error("al quitar el secreto anterior, los tokens viejos dejan de valer")
	}
}

func TestWeakSecrets(t *testing.T) {
	if _, err := New("corto", "", DefaultTTL, time.Now); !errors.Is(err, ErrWeakSecret) {
		t.Errorf("secreto corto = %v", err)
	}
	if _, err := New(secretA, "corto", DefaultTTL, time.Now); !errors.Is(err, ErrWeakSecret) {
		t.Errorf("secreto anterior corto = %v", err)
	}
}
