package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/whiteowltech/owli/backend/internal/adapters/memory"
	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

type fakeIssuer struct{}

func (fakeIssuer) Issue(u domain.UserID, now time.Time) (string, time.Time, error) {
	return "access-" + string(u), now.Add(15 * time.Minute), nil
}

// fakeVerifier acepta tokens "ok:<subject>".
type fakeVerifier struct{ provider ports.IdentityProvider }

func (f fakeVerifier) Verify(_ context.Context, token, _ string) (ports.ExternalIdentity, error) {
	if len(token) < 4 || token[:3] != "ok:" {
		return ports.ExternalIdentity{}, ports.ErrInvalidCredentials
	}
	return ports.ExternalIdentity{Provider: f.provider, Subject: token[3:]}, nil
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newAuth() (*AuthService, *memory.Store, *clock) {
	users := memory.NewStore()
	c := &clock{t: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	verifiers := map[ports.IdentityProvider]ports.IdentityVerifier{ports.ProviderApple: fakeVerifier{ports.ProviderApple}}
	return NewAuthService(users, users, fakeIssuer{}, verifiers, DefaultRefreshTTL, c.now), users, c
}

// ---- Tests ----

func TestCreateAnonymousAndRefresh(t *testing.T) {
	ctx := context.Background()
	auth, _, _ := newAuth()

	s1, err := auth.CreateAnonymous(ctx)
	if err != nil {
		t.Fatalf("CreateAnonymous: %v", err)
	}
	if s1.AccessToken == "" || s1.RefreshToken == "" || s1.UserID == "" {
		t.Fatalf("sesión incompleta: %+v", s1)
	}

	s2, err := auth.Refresh(ctx, s1.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if s2.UserID != s1.UserID || s2.RefreshToken == s1.RefreshToken {
		t.Fatalf("la renovación debe mantener el usuario y rotar el token")
	}
}

func TestRefreshReuseRevokesTheWholeFamily(t *testing.T) {
	ctx := context.Background()
	auth, _, _ := newAuth()
	s1, _ := auth.CreateAnonymous(ctx)
	s2, _ := auth.Refresh(ctx, s1.RefreshToken)

	// Alguien reusa el token viejo (copia robada): se cierra la familia.
	if _, err := auth.Refresh(ctx, s1.RefreshToken); !errors.Is(err, ports.ErrSessionReused) {
		t.Fatalf("reuso = %v, se esperaba ErrSessionReused", err)
	}
	// Y el token legítimo más nuevo también deja de servir.
	if _, err := auth.Refresh(ctx, s2.RefreshToken); !errors.Is(err, ports.ErrSessionReused) {
		t.Fatalf("token nuevo tras el reuso = %v, se esperaba ErrSessionReused", err)
	}
}

func TestRefreshErrors(t *testing.T) {
	ctx := context.Background()
	auth, _, c := newAuth()
	s1, _ := auth.CreateAnonymous(ctx)

	if _, err := auth.Refresh(ctx, "no-es-un-token"); !errors.Is(err, ports.ErrInvalidCredentials) {
		t.Errorf("token mal formado = %v", err)
	}
	unknown, _, _ := newRefreshToken()
	if _, err := auth.Refresh(ctx, unknown); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Errorf("token desconocido = %v", err)
	}
	c.t = c.t.Add(DefaultRefreshTTL + time.Second)
	if _, err := auth.Refresh(ctx, s1.RefreshToken); !errors.Is(err, ports.ErrSessionExpired) {
		t.Errorf("token vencido = %v", err)
	}
}

func TestLogout(t *testing.T) {
	ctx := context.Background()
	auth, _, _ := newAuth()
	s1, _ := auth.CreateAnonymous(ctx)
	if err := auth.Logout(ctx, s1.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := auth.Refresh(ctx, s1.RefreshToken); err == nil {
		t.Fatal("después del logout el refresh token no debe servir")
	}
	if err := auth.Logout(ctx, "basura"); err != nil {
		t.Errorf("logout con token inválido no debe fallar: %v", err)
	}
}

func TestSignInCreatesOnceAndLinksAnonymous(t *testing.T) {
	ctx := context.Background()
	auth, _, _ := newAuth()

	first, created, err := auth.SignIn(ctx, ports.ProviderApple, "ok:apple-123", "")
	if err != nil || !created {
		t.Fatalf("primer SignIn: created=%v err=%v", created, err)
	}
	again, created, err := auth.SignIn(ctx, ports.ProviderApple, "ok:apple-123", "")
	if err != nil || created || again.UserID != first.UserID {
		t.Fatalf("segundo SignIn debe entrar al mismo usuario: %v %v %v", again.UserID, created, err)
	}

	// Cuenta anónima que luego vincula Apple: conserva su UserID.
	anon, _ := auth.CreateAnonymous(ctx)
	if err := auth.Link(ctx, anon.UserID, ports.ProviderApple, "ok:apple-999", ""); err != nil {
		t.Fatalf("Link: %v", err)
	}
	back, created, _ := auth.SignIn(ctx, ports.ProviderApple, "ok:apple-999", "")
	if created || back.UserID != anon.UserID {
		t.Fatalf("tras vincular, SignIn debe volver a la cuenta anónima")
	}
	// Vincular de nuevo la misma identidad es idempotente.
	if err := auth.Link(ctx, anon.UserID, ports.ProviderApple, "ok:apple-999", ""); err != nil {
		t.Errorf("re-vincular la misma identidad: %v", err)
	}
	// La identidad de otra cuenta no se puede robar.
	if err := auth.Link(ctx, anon.UserID, ports.ProviderApple, "ok:apple-123", ""); !errors.Is(err, ports.ErrIdentityInUse) {
		t.Errorf("vincular identidad ajena = %v, se esperaba ErrIdentityInUse", err)
	}
}

func TestSignInErrors(t *testing.T) {
	ctx := context.Background()
	auth, _, _ := newAuth()
	if _, _, err := auth.SignIn(ctx, ports.ProviderApple, "falso", ""); !errors.Is(err, ports.ErrInvalidCredentials) {
		t.Errorf("token inválido = %v", err)
	}
	if _, _, err := auth.SignIn(ctx, ports.ProviderGoogle, "ok:g", ""); !errors.Is(err, ports.ErrProviderNotConfigured) {
		t.Errorf("proveedor sin configurar = %v", err)
	}
}

func TestDeleteAccountFreesTheIdentity(t *testing.T) {
	ctx := context.Background()
	auth, users, _ := newAuth()
	s, _, _ := auth.SignIn(ctx, ports.ProviderApple, "ok:apple-1", "")
	if err := auth.DeleteAccount(ctx, s.UserID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if users.HasUser(s.UserID) {
		t.Fatal("el usuario debería estar borrado")
	}
	if _, created, _ := auth.SignIn(ctx, ports.ProviderApple, "ok:apple-1", ""); !created {
		t.Error("tras borrar la cuenta, la misma identidad crea una cuenta nueva")
	}
}
