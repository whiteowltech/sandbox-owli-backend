package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/whiteowltech/owli/backend/internal/adapters/postgres/db"
	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

// welcomeReference: la bienvenida se da una sola vez por usuario (clave única).
const welcomeReference = "welcome"

// Credits implementa ports.CreditStore sobre el ledger append-only.
type Credits struct{ s *Store }

func (s *Store) Credits() *Credits { return &Credits{s: s} }

func day(localDay string) (pgtype.Date, error) {
	t, err := time.Parse("2006-01-02", localDay)
	if err != nil {
		return pgtype.Date{}, fmt.Errorf("%w: día %q", domain.ErrInvalidProfile, localDay)
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

// inUserTx corre fn en una transacción con la fila del usuario bloqueada.
func (c *Credits) inUserTx(ctx context.Context, userID domain.UserID, fn func(q *db.Queries) error) error {
	uid, err := parseUserID(userID)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, c.s.pool, func(tx pgx.Tx) error {
		q := c.s.q.WithTx(tx)
		if _, err := q.LockUser(ctx, uid); errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: usuario", ports.ErrNotFound)
		} else if err != nil {
			return fmt.Errorf("bloqueando usuario: %w", err)
		}
		return fn(q)
	})
}

func (c *Credits) ensureWelcomeAndSum(ctx context.Context, q *db.Queries, userID domain.UserID, d pgtype.Date, welcome int) (domain.CreditWallet, error) {
	uid, _ := parseUserID(userID)
	if welcome > 0 {
		if _, err := q.InsertCreditTx(ctx, db.InsertCreditTxParams{
			UserID: uid, Kind: string(domain.KindGrant), Bucket: string(domain.BucketWelcome),
			Delta: int32(welcome), Reference: welcomeReference, LocalDay: d,
		}); err != nil {
			return domain.CreditWallet{}, fmt.Errorf("dando créditos de bienvenida: %w", err)
		}
	}
	sums, err := q.WalletSums(ctx, db.WalletSumsParams{UserID: uid, LocalDay: d})
	if err != nil {
		return domain.CreditWallet{}, fmt.Errorf("calculando saldo: %w", err)
	}
	// IsPro llegará con RevenueCat (ver pendientes.md).
	return domain.CreditWallet{UsedToday: int(sums.UsedToday), WelcomeBalance: int(sums.WelcomeBalance), PackBalance: int(sums.PackBalance)}, nil
}

func (c *Credits) Wallet(ctx context.Context, userID domain.UserID, localDay string, welcome int) (domain.CreditWallet, error) {
	d, err := day(localDay)
	if err != nil {
		return domain.CreditWallet{}, err
	}
	var w domain.CreditWallet
	err = c.inUserTx(ctx, userID, func(q *db.Queries) error {
		w, err = c.ensureWelcomeAndSum(ctx, q, userID, d, welcome)
		return err
	})
	return w, err
}

func (c *Credits) Debit(ctx context.Context, userID domain.UserID, localDay, reference string, welcome int, pick ports.PickBucket) (domain.CreditBucket, domain.CreditWallet, error) {
	d, err := day(localDay)
	if err != nil {
		return "", domain.CreditWallet{}, err
	}
	uid, err := parseUserID(userID)
	if err != nil {
		return "", domain.CreditWallet{}, err
	}
	var bucket domain.CreditBucket
	var after domain.CreditWallet
	err = c.inUserTx(ctx, userID, func(q *db.Queries) error {
		w, err := c.ensureWelcomeAndSum(ctx, q, userID, d, welcome)
		if err != nil {
			return err
		}
		if bucket, err = pick(w); err != nil {
			return err // sin créditos: no se escribe nada
		}
		n, err := q.InsertCreditTx(ctx, db.InsertCreditTxParams{
			UserID: uid, Kind: string(domain.KindConsume), Bucket: string(bucket), Delta: -1, Reference: reference, LocalDay: d,
		})
		if err != nil {
			return fmt.Errorf("registrando consumo: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("consumo %q duplicado", reference)
		}
		after, err = c.ensureWelcomeAndSum(ctx, q, userID, d, 0)
		return err
	})
	if err != nil {
		return "", domain.CreditWallet{}, err
	}
	return bucket, after, nil
}

func (c *Credits) Refund(ctx context.Context, userID domain.UserID, reference string) error {
	uid, err := parseUserID(userID)
	if err != nil {
		return err
	}
	return c.inUserTx(ctx, userID, func(q *db.Queries) error {
		consume, err := q.GetConsume(ctx, db.GetConsumeParams{UserID: uid, Reference: reference})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("buscando consumo: %w", err)
		}
		// Mismo bucket y mismo día local que el consumo: así la cuota diaria se restituye bien.
		if _, err := q.InsertCreditTx(ctx, db.InsertCreditTxParams{
			UserID: uid, Kind: string(domain.KindRefund), Bucket: consume.Bucket, Delta: 1, Reference: reference, LocalDay: consume.LocalDay,
		}); err != nil {
			return fmt.Errorf("registrando reembolso: %w", err)
		}
		return nil
	})
}

var _ ports.CreditStore = (*Credits)(nil)
