package postgres

// Tests de integración contra un Postgres real. Se saltean si no hay base:
//   TEST_DATABASE_URL=postgres://owli:owli@localhost:5433/owli_test?sslmode=disable go test ./...
// (docker compose up -d desde la raíz; la base owli_test se usa solo para tests y se vacía en cada uno.)

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
	"github.com/whiteowltech/owli/backend/migrations"
)

func testStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL no definida: se saltean los tests de integración de Postgres")
	}
	ctx := context.Background()
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE users CASCADE"); err != nil {
		t.Fatalf("limpiando: %v", err)
	}
	return NewStore(pool), pool
}

func hash(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

var t0 = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

func TestMigrateIsIdempotent(t *testing.T) {
	_, pool := testStore(t)
	for range 2 {
		if err := Migrate(context.Background(), pool, migrations.FS); err != nil {
			t.Fatalf("segunda migración: %v", err)
		}
	}
}

func TestIdentities(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	u1, _ := s.CreateUser(ctx)
	u2, _ := s.CreateUser(ctx)
	apple := ports.ExternalIdentity{Provider: ports.ProviderApple, Subject: "001.abc", Email: "x@privaterelay.appleid.com"}

	if _, err := s.FindByIdentity(ctx, ports.ProviderApple, "001.abc"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("antes de vincular = %v", err)
	}
	if err := s.LinkIdentity(ctx, u1, apple); err != nil {
		t.Fatalf("LinkIdentity: %v", err)
	}
	if err := s.LinkIdentity(ctx, u1, apple); err != nil {
		t.Errorf("re-vincular al mismo usuario debe ser idempotente: %v", err)
	}
	if err := s.LinkIdentity(ctx, u2, apple); !errors.Is(err, ports.ErrIdentityInUse) {
		t.Errorf("vincular identidad ajena = %v", err)
	}
	if got, _ := s.FindByIdentity(ctx, ports.ProviderApple, "001.abc"); got != u1 {
		t.Errorf("dueño = %v, se esperaba %v", got, u1)
	}
	if err := s.DeleteUser(ctx, u1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindByIdentity(ctx, ports.ProviderApple, "001.abc"); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("al borrar el usuario se borran sus identidades: %v", err)
	}
}

func TestSessionRotation(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx)
	if err := s.Create(ctx, u, hash("r1"), t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, err := s.Rotate(ctx, hash("r1"), hash("r2"), t0.Add(2*time.Hour), t0)
	if err != nil || got != u {
		t.Fatalf("Rotate = %v, %v", got, err)
	}
	if _, err := s.Rotate(ctx, hash("nada"), hash("x"), t0, t0); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Errorf("token desconocido = %v", err)
	}
	// Reuso de r1: se revoca la familia, incluido r2.
	if _, err := s.Rotate(ctx, hash("r1"), hash("r3"), t0, t0); !errors.Is(err, ports.ErrSessionReused) {
		t.Fatalf("reuso = %v", err)
	}
	if _, err := s.Rotate(ctx, hash("r2"), hash("r4"), t0, t0); !errors.Is(err, ports.ErrSessionReused) {
		t.Errorf("r2 tras el reuso = %v (la familia debía quedar revocada)", err)
	}
}

func TestSessionExpiryAndLogout(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx)
	_ = s.Create(ctx, u, hash("e1"), t0)
	if _, err := s.Rotate(ctx, hash("e1"), hash("e2"), t0, t0); !errors.Is(err, ports.ErrSessionExpired) {
		t.Errorf("vencido = %v", err)
	}

	_ = s.Create(ctx, u, hash("l1"), t0.Add(time.Hour))
	if err := s.Revoke(ctx, hash("l1"), t0); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(ctx, hash("no-existe"), t0); err != nil {
		t.Errorf("revocar un token inexistente no falla: %v", err)
	}
	if _, err := s.Rotate(ctx, hash("l1"), hash("l2"), t0.Add(time.Hour), t0); err == nil {
		t.Error("un token revocado no se puede renovar")
	}
}

// Dos renovaciones simultáneas del mismo token: solo una puede ganar.
func TestConcurrentRotationOnlyOneWins(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx)
	_ = s.Create(ctx, u, hash("c1"), t0.Add(time.Hour))

	var wg sync.WaitGroup
	results := make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = s.Rotate(ctx, hash("c1"), hash(fmt.Sprintf("c-new-%d", i)), t0.Add(2*time.Hour), t0)
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range results {
		if err == nil {
			ok++
		} else if !errors.Is(err, ports.ErrSessionReused) {
			t.Errorf("error inesperado: %v", err)
		}
	}
	if ok != 1 {
		t.Errorf("renovaciones exitosas = %d, se esperaba exactamente 1", ok)
	}
}

func TestProfileRoundTrip(t *testing.T) {
	s, pool := testStore(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx)
	if _, err := s.Get(ctx, u); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("sin perfil = %v", err)
	}
	p := domain.UserProfile{
		Name: "Leandro",
		Bio:  domain.Biometrics{WeightKg: 80.5, HeightCm: 175, AgeYears: 30, Sex: domain.SexMale, Activity: domain.ActivityModerate},
		Goal: domain.GoalCut, Pace: domain.PaceFast, TargetWeightKg: 74.5,
		Diet: domain.DietVegan, Restrictions: []domain.Restriction{domain.RestrictionGlutenFree, domain.RestrictionLactoseFree},
		MealsPerDay: 3, Units: domain.UnitsImperial, Timezone: "America/Argentina/Buenos_Aires",
	}
	if err := s.Upsert(ctx, u, p); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := s.Get(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != fmt.Sprint(p) {
		t.Errorf("leído:\n%+v\nguardado:\n%+v", got, p)
	}
	p.Diet, p.Restrictions = domain.DietKeto, []domain.Restriction{}
	_ = s.Upsert(ctx, u, p)
	got, _ = s.Get(ctx, u)
	if got.Diet != domain.DietKeto || len(got.Restrictions) != 0 {
		t.Errorf("la actualización no se aplicó: %+v", got)
	}

	// La base también rechaza datos imposibles aunque se saltee el dominio.
	p.Bio.AgeYears = 15
	if err := s.Upsert(ctx, u, p); err == nil {
		t.Error("la restricción CHECK de edad debía rechazar 15 años")
	}
	var n int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM profiles").Scan(&n)
	if n != 1 {
		t.Errorf("perfiles = %d", n)
	}
}

func TestUpsertForDeletedUserIsNotFound(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx)
	_ = s.DeleteUser(ctx, u)
	p := domain.UserProfile{Bio: domain.Biometrics{WeightKg: 70, HeightCm: 170, AgeYears: 30, Sex: domain.SexFemale, Activity: domain.ActivityLight},
		Goal: domain.GoalMaintain, Pace: domain.PaceSteady, TargetWeightKg: 70, Diet: domain.DietOmnivore, MealsPerDay: 3,
		Units: domain.UnitsMetric, Timezone: "UTC"}
	if err := s.Upsert(ctx, u, p); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("perfil de usuario borrado = %v, se esperaba ErrNotFound", err)
	}
}

func TestUnknownUserIDIsNotFound(t *testing.T) {
	s, _ := testStore(t)
	if _, err := s.Get(context.Background(), "no-es-uuid"); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("Get con id inválido = %v", err)
	}
}
