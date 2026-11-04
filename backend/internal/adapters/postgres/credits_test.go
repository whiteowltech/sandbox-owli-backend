package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/whiteowltech/owli/backend/internal/domain"
)

var policy = domain.CreditPolicy{FreeDailyQuota: 1, WelcomeCredits: 15, ProFairUseDaily: 25}

func TestCreditsOrderAndRefund(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	c := s.Credits()
	u, _ := s.CreateUser(ctx)

	w, err := c.Wallet(ctx, u, "2026-11-03", 15)
	if err != nil || w.WelcomeBalance != 15 || w.UsedToday != 0 {
		t.Fatalf("billetera inicial = %+v, %v", w, err)
	}
	if w, _ = c.Wallet(ctx, u, "2026-11-03", 15); w.WelcomeBalance != 15 {
		t.Error("la bienvenida se da una sola vez")
	}

	// 1.º del día: cuota diaria. 2.º: bienvenida.
	b1, w1, err := c.Debit(ctx, u, "2026-11-03", "a1", 15, policy.PickDebitBucket)
	if err != nil || b1 != domain.BucketDailyQuota || w1.UsedToday != 1 || w1.WelcomeBalance != 15 {
		t.Fatalf("primer consumo = %s %+v %v", b1, w1, err)
	}
	b2, w2, _ := c.Debit(ctx, u, "2026-11-03", "a2", 15, policy.PickDebitBucket)
	if b2 != domain.BucketWelcome || w2.WelcomeBalance != 14 {
		t.Fatalf("segundo consumo = %s %+v", b2, w2)
	}

	// Reembolso al mismo bucket, idempotente.
	for range 2 {
		if err := c.Refund(ctx, u, "a1"); err != nil {
			t.Fatal(err)
		}
	}
	if w, _ = c.Wallet(ctx, u, "2026-11-03", 15); w.UsedToday != 0 || w.WelcomeBalance != 14 {
		t.Errorf("tras reembolsar la cuota diaria = %+v", w)
	}
	if err := c.Refund(ctx, u, "nunca-existio"); err != nil {
		t.Errorf("reembolsar algo inexistente no falla: %v", err)
	}

	// Al día siguiente vuelve la cuota diaria.
	if b, _, _ := c.Debit(ctx, u, "2026-11-04", "a3", 15, policy.PickDebitBucket); b != domain.BucketDailyQuota {
		t.Errorf("día nuevo = %s, se esperaba cuota diaria", b)
	}
	if _, _, err := c.Debit(ctx, u, "2026-11-04", "a3", 15, policy.PickDebitBucket); err == nil {
		t.Error("la misma referencia no se cobra dos veces")
	}
}

func TestCreditsRunOut(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	c := s.Credits()
	u, _ := s.CreateUser(ctx)
	for i := range 1 + 15 {
		if _, _, err := c.Debit(ctx, u, "2026-11-03", fmt.Sprintf("r%d", i), 15, policy.PickDebitBucket); err != nil {
			t.Fatalf("consumo %d: %v", i+1, err)
		}
	}
	if _, _, err := c.Debit(ctx, u, "2026-11-03", "r-extra", 15, policy.PickDebitBucket); !errors.Is(err, domain.ErrInsufficientCredits) {
		t.Errorf("sin créditos = %v", err)
	}
}

// Muchos análisis a la vez: nunca se gastan más créditos de los que hay.
func TestConcurrentDebitsNeverOverspend(t *testing.T) {
	s, pool := testStore(t)
	ctx := context.Background()
	c := s.Credits()
	u, _ := s.CreateUser(ctx)

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := c.Debit(ctx, u, "2026-11-03", fmt.Sprintf("c%d", i), 15, policy.PickDebitBucket); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 16 {
		t.Errorf("consumos exitosos = %d, se esperaba exactamente 16 (1 diario + 15 de bienvenida)", ok)
	}
	var welcomeGrants int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM credit_transactions WHERE kind = 'grant'").Scan(&welcomeGrants)
	if welcomeGrants != 1 {
		t.Errorf("bienvenidas = %d, se esperaba 1", welcomeGrants)
	}
}

func TestLedgerIsAppendOnly(t *testing.T) {
	s, pool := testStore(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx)
	_, _ = s.Credits().Wallet(ctx, u, "2026-11-03", 15)
	if _, err := pool.Exec(ctx, "UPDATE credit_transactions SET delta = 1000"); err == nil {
		t.Fatal("un UPDATE del ledger debe fallar")
	}
	// Borrar la cuenta sí borra sus movimientos (eliminación de datos).
	if err := s.DeleteUser(ctx, u); err != nil {
		t.Fatalf("borrar la cuenta: %v", err)
	}
}
