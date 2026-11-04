package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

type creditTx struct {
	user   domain.UserID
	kind   domain.CreditKind
	bucket domain.CreditBucket
	delta  int
	ref    string
	day    string
}

// Credits es el libro de créditos en memoria (mismas reglas que el de Postgres).
type Credits struct {
	mu  sync.Mutex
	txs []creditTx
}

func NewCredits() *Credits { return &Credits{} }

func (c *Credits) find(u domain.UserID, kind domain.CreditKind, ref string) *creditTx {
	for i := range c.txs {
		if t := &c.txs[i]; t.user == u && t.kind == kind && t.ref == ref {
			return t
		}
	}
	return nil
}

func (c *Credits) walletLocked(u domain.UserID, day string, welcome int) domain.CreditWallet {
	if welcome > 0 && c.find(u, domain.KindGrant, "welcome") == nil {
		c.txs = append(c.txs, creditTx{user: u, kind: domain.KindGrant, bucket: domain.BucketWelcome, delta: welcome, ref: "welcome", day: day})
	}
	var w domain.CreditWallet
	for _, t := range c.txs {
		if t.user != u {
			continue
		}
		switch t.bucket {
		case domain.BucketWelcome:
			w.WelcomeBalance += t.delta
		case domain.BucketPack:
			w.PackBalance += t.delta
		case domain.BucketDailyQuota, domain.BucketSubscription:
			if t.day == day {
				w.UsedToday -= t.delta
			}
		}
	}
	return w
}

func (c *Credits) Wallet(_ context.Context, u domain.UserID, day string, welcome int) (domain.CreditWallet, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.walletLocked(u, day, welcome), nil
}

func (c *Credits) Debit(_ context.Context, u domain.UserID, day, ref string, welcome int, pick ports.PickBucket) (domain.CreditBucket, domain.CreditWallet, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, err := pick(c.walletLocked(u, day, welcome))
	if err != nil {
		return "", domain.CreditWallet{}, err
	}
	if c.find(u, domain.KindConsume, ref) != nil {
		return "", domain.CreditWallet{}, fmt.Errorf("consumo %q duplicado", ref)
	}
	c.txs = append(c.txs, creditTx{user: u, kind: domain.KindConsume, bucket: b, delta: -1, ref: ref, day: day})
	return b, c.walletLocked(u, day, 0), nil
}

func (c *Credits) Refund(_ context.Context, u domain.UserID, ref string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	consume := c.find(u, domain.KindConsume, ref)
	if consume == nil || c.find(u, domain.KindRefund, ref) != nil {
		return nil
	}
	c.txs = append(c.txs, creditTx{user: u, kind: domain.KindRefund, bucket: consume.bucket, delta: 1, ref: ref, day: consume.day})
	return nil
}

var _ ports.CreditStore = (*Credits)(nil)
