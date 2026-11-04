// Package postgres implementa los repositorios con pgx y el código generado por
// sqlc (internal/adapters/postgres/db). Nada de SQL concatenado.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whiteowltech/owli/backend/internal/adapters/postgres/db"
	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

// Connect abre el pool y verifica la conexión.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("configurando Postgres: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("conectando a Postgres: %w", err)
	}
	return pool, nil
}

// migrationLockID es una clave arbitraria del advisory lock: si arrancan dos
// instancias a la vez, solo una aplica migraciones.
const migrationLockID = 727_465_100

// Migrate aplica en orden los archivos .sql que todavía no se aplicaron. Cada
// migración corre en su propia transacción.
func Migrate(ctx context.Context, pool *pgxpool.Pool, files fs.FS) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migraciones: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("migraciones: tomando el lock: %w", err)
	}
	defer conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", migrationLockID) //nolint:errcheck // se libera igual al cerrar la conexión

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("migraciones: tabla de control: %w", err)
	}
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return fmt.Errorf("migraciones: listando archivos: %w", err)
	}
	sort.Strings(names)
	for _, name := range names {
		var exists bool
		if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", name).Scan(&exists); err != nil {
			return fmt.Errorf("migraciones: consultando %s: %w", name, err)
		}
		if exists {
			continue
		}
		sql, err := fs.ReadFile(files, name)
		if err != nil {
			return fmt.Errorf("migraciones: leyendo %s: %w", name, err)
		}
		if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", name)
			return err
		}); err != nil {
			return fmt.Errorf("migraciones: aplicando %s: %w", name, err)
		}
	}
	return nil
}

// Store implementa UserRepository, SessionStore y ProfileRepository.
type Store struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: db.New(pool)}
}

func parseUserID(id domain.UserID) (uuid.UUID, error) {
	u, err := uuid.Parse(string(id))
	if err != nil {
		// Un UserID que no es UUID no puede existir: se trata como inexistente.
		return uuid.Nil, fmt.Errorf("%w: usuario %q", ports.ErrNotFound, id)
	}
	return u, nil
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// ---- UserRepository ----

func (s *Store) CreateUser(ctx context.Context) (domain.UserID, error) {
	id, err := s.q.CreateUser(ctx)
	if err != nil {
		return "", fmt.Errorf("insertando usuario: %w", err)
	}
	return domain.UserID(id.String()), nil
}

func (s *Store) FindByIdentity(ctx context.Context, provider ports.IdentityProvider, subject string) (domain.UserID, error) {
	id, err := s.q.FindUserByIdentity(ctx, db.FindUserByIdentityParams{Provider: string(provider), Subject: subject})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ports.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("buscando identidad: %w", err)
	}
	return domain.UserID(id.String()), nil
}

func (s *Store) LinkIdentity(ctx context.Context, userID domain.UserID, ext ports.ExternalIdentity) error {
	uid, err := parseUserID(userID)
	if err != nil {
		return err
	}
	n, err := s.q.InsertIdentity(ctx, db.InsertIdentityParams{
		Provider: string(ext.Provider), Subject: ext.Subject, UserID: uid,
		Email: pgtype.Text{String: ext.Email, Valid: ext.Email != ""},
	})
	if err != nil {
		return fmt.Errorf("insertando identidad: %w", err)
	}
	if n == 1 {
		return nil
	}
	// Ya existía: idempotente si es del mismo usuario, conflicto si es de otro.
	owner, err := s.FindByIdentity(ctx, ext.Provider, ext.Subject)
	if err != nil {
		return err
	}
	if owner != userID {
		return ports.ErrIdentityInUse
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, userID domain.UserID) error {
	uid, err := parseUserID(userID)
	if err != nil {
		return err
	}
	if _, err := s.q.DeleteUser(ctx, uid); err != nil {
		return fmt.Errorf("borrando usuario: %w", err)
	}
	return nil
}

// ---- SessionStore ----

func (s *Store) Create(ctx context.Context, userID domain.UserID, tokenHash []byte, expiresAt time.Time) error {
	uid, err := parseUserID(userID)
	if err != nil {
		return err
	}
	if err := s.q.InsertSession(ctx, db.InsertSessionParams{UserID: uid, FamilyID: uuid.New(), TokenHash: tokenHash, ExpiresAt: ts(expiresAt)}); err != nil {
		return fmt.Errorf("insertando sesión: %w", err)
	}
	return nil
}

// Rotate implementa las reglas documentadas en ports.SessionStore dentro de una
// transacción con la fila bloqueada (SELECT … FOR UPDATE).
func (s *Store) Rotate(ctx context.Context, oldHash, newHash []byte, newExpiresAt, now time.Time) (domain.UserID, error) {
	var userID domain.UserID
	var outcome error
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		row, err := q.LockSessionByHash(ctx, oldHash)
		if errors.Is(err, pgx.ErrNoRows) {
			outcome = ports.ErrSessionNotFound
			return nil
		}
		if err != nil {
			return fmt.Errorf("bloqueando sesión: %w", err)
		}
		if row.UsedAt.Valid || row.RevokedAt.Valid {
			// Se confirma la revocación aunque el resultado sea un error de negocio.
			if err := q.RevokeSessionFamily(ctx, db.RevokeSessionFamilyParams{FamilyID: row.FamilyID, RevokedAt: ts(now)}); err != nil {
				return fmt.Errorf("revocando familia: %w", err)
			}
			outcome = ports.ErrSessionReused
			return nil
		}
		if !now.Before(row.ExpiresAt.Time) {
			outcome = ports.ErrSessionExpired
			return nil
		}
		if err := q.MarkSessionUsed(ctx, db.MarkSessionUsedParams{ID: row.ID, UsedAt: ts(now)}); err != nil {
			return fmt.Errorf("marcando sesión usada: %w", err)
		}
		if err := q.InsertSession(ctx, db.InsertSessionParams{UserID: row.UserID, FamilyID: row.FamilyID, TokenHash: newHash, ExpiresAt: ts(newExpiresAt)}); err != nil {
			return fmt.Errorf("insertando sesión rotada: %w", err)
		}
		userID = domain.UserID(row.UserID.String())
		return nil
	})
	if err != nil {
		return "", err
	}
	if outcome != nil {
		return "", outcome
	}
	return userID, nil
}

func (s *Store) Revoke(ctx context.Context, tokenHash []byte, now time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		row, err := q.LockSessionByHash(ctx, tokenHash)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("bloqueando sesión: %w", err)
		}
		if err := q.RevokeSessionFamily(ctx, db.RevokeSessionFamilyParams{FamilyID: row.FamilyID, RevokedAt: ts(now)}); err != nil {
			return fmt.Errorf("revocando familia: %w", err)
		}
		return nil
	})
}

// ---- ProfileRepository ----

func (s *Store) Get(ctx context.Context, userID domain.UserID) (domain.UserProfile, error) {
	uid, err := parseUserID(userID)
	if err != nil {
		return domain.UserProfile{}, err
	}
	r, err := s.q.GetProfile(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UserProfile{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("leyendo perfil: %w", err)
	}
	restr := make([]domain.Restriction, len(r.Restrictions))
	for i, x := range r.Restrictions {
		restr[i] = domain.Restriction(x)
	}
	return domain.UserProfile{
		Name: r.Name,
		Bio: domain.Biometrics{
			WeightKg: r.WeightKg, HeightCm: r.HeightCm, AgeYears: int(r.AgeYears),
			Sex: domain.Sex(r.Sex), Activity: domain.ActivityLevel(r.Activity),
		},
		Goal: domain.Goal(r.Goal), Pace: domain.Pace(r.Pace), TargetWeightKg: r.TargetWeightKg,
		Diet: domain.DietType(r.Diet), Restrictions: restr, MealsPerDay: int(r.MealsPerDay),
		Units: domain.UnitSystem(r.Units), Timezone: r.Timezone,
	}, nil
}

func (s *Store) Upsert(ctx context.Context, userID domain.UserID, p domain.UserProfile) error {
	uid, err := parseUserID(userID)
	if err != nil {
		return err
	}
	restr := make([]string, len(p.Restrictions))
	for i, x := range p.Restrictions {
		restr[i] = string(x)
	}
	err = s.q.UpsertProfile(ctx, db.UpsertProfileParams{
		UserID: uid, Name: p.Name, Sex: string(p.Bio.Sex), AgeYears: int16(p.Bio.AgeYears),
		HeightCm: p.Bio.HeightCm, WeightKg: p.Bio.WeightKg, Activity: string(p.Bio.Activity),
		Goal: string(p.Goal), Pace: string(p.Pace), TargetWeightKg: p.TargetWeightKg, Diet: string(p.Diet),
		Restrictions: restr, MealsPerDay: int16(p.MealsPerDay), Units: string(p.Units), Timezone: p.Timezone,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ForeignKeyViolation {
		// El usuario ya no existe (p. ej. borró la cuenta con un access token aún vigente).
		return fmt.Errorf("%w: usuario %s", ports.ErrNotFound, userID)
	}
	if err != nil {
		return fmt.Errorf("guardando perfil: %w", err)
	}
	return nil
}

// pgerrcode: códigos SQLSTATE usados por este adapter.
var pgerrcode = struct{ ForeignKeyViolation string }{ForeignKeyViolation: "23503"}
