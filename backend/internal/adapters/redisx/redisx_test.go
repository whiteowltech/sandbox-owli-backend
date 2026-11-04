package redisx

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/whiteowltech/owli/backend/internal/ports"
)

// miniredis es un Redis en memoria que ejecuta los mismos scripts Lua.
func newRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	return mr, redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestRateLimiterWindow(t *testing.T) {
	mr, rdb := newRedis(t)
	l := NewRateLimiter(rdb)
	ctx := context.Background()

	for i := range 3 {
		if ok, _, err := l.Allow(ctx, "ip:1.2.3.4", 3, time.Hour); !ok || err != nil {
			t.Fatalf("pedido %d bloqueado: %v", i+1, err)
		}
	}
	ok, retry, err := l.Allow(ctx, "ip:1.2.3.4", 3, time.Hour)
	if ok || err != nil || retry <= 0 || retry > time.Hour {
		t.Fatalf("el 4.º pedido debía bloquearse con retryAfter: ok=%v retry=%v err=%v", ok, retry, err)
	}
	if ok, _, _ := l.Allow(ctx, "ip:5.6.7.8", 3, time.Hour); !ok {
		t.Error("otra IP tiene su propio contador")
	}
	mr.FastForward(time.Hour + time.Second)
	if ok, _, _ := l.Allow(ctx, "ip:1.2.3.4", 3, time.Hour); !ok {
		t.Error("tras la ventana, se habilita de nuevo")
	}
}

func TestRateLimiterRedisDown(t *testing.T) {
	mr, rdb := newRedis(t)
	mr.Close()
	_, _, err := NewRateLimiter(rdb).Allow(context.Background(), "k", 1, time.Minute)
	if !errors.Is(err, ports.ErrDependencyUnavailable) {
		t.Errorf("Redis caído = %v", err)
	}
}

var day = time.Date(2026, 11, 3, 23, 0, 0, 0, time.UTC)

func TestSpendGuardCap(t *testing.T) {
	mr, rdb := newRedis(t)
	g := NewSpendGuard(rdb, 0.01) // 10.000 micro-USD
	ctx := context.Background()

	for i := range 3 {
		if err := g.Reserve(ctx, 3000, day); err != nil {
			t.Fatalf("reserva %d: %v", i+1, err)
		}
	}
	if err := g.Reserve(ctx, 3000, day); !errors.Is(err, ports.ErrAIBudgetExceeded) {
		t.Fatalf("superar el tope = %v", err)
	}
	if v, _ := mr.Get(spendKey(day)); v != "9000" {
		t.Errorf("la reserva rechazada no debe sumar: total %s", v)
	}
	// Un análisis falló: se libera y vuelve a haber lugar.
	_ = g.Release(ctx, 3000, day)
	if err := g.Reserve(ctx, 3000, day); err != nil {
		t.Errorf("tras liberar: %v", err)
	}
	// Al día siguiente (UTC) el contador arranca de cero.
	if err := g.Reserve(ctx, 9000, day.Add(2*time.Hour)); err != nil {
		t.Errorf("día nuevo: %v", err)
	}
}

func TestSpendGuardConcurrentNeverExceedsCap(t *testing.T) {
	mr, rdb := newRedis(t)
	g := NewSpendGuard(rdb, 0.01)
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = g.Reserve(context.Background(), 1000, day)
		}()
	}
	wg.Wait()
	if v, _ := mr.Get(spendKey(day)); v != "10000" {
		t.Errorf("total = %s, se esperaba exactamente el tope (10000)", v)
	}
}

func TestSpendGuardFailsClosed(t *testing.T) {
	mr, rdb := newRedis(t)
	mr.Close()
	if err := NewSpendGuard(rdb, 10).Reserve(context.Background(), 1, day); !errors.Is(err, ports.ErrDependencyUnavailable) {
		t.Errorf("Redis caído: la IA no debe usarse sin control de gasto, got %v", err)
	}
}
