// Package redisx implementa en Redis el límite de solicitudes y el tope diario
// de gasto en IA.
package redisx

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/whiteowltech/owli/backend/internal/ports"
)

// Connect abre el cliente y verifica la conexión.
func Connect(ctx context.Context, url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("REDIS_URL inválida: %w", err)
	}
	c := redis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := c.Ping(pingCtx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("conectando a Redis: %w", err)
	}
	return c, nil
}

// RateLimiter es una ventana fija: cuenta solicitudes por clave y ventana.
// Simple y suficiente contra bots; no busca precisión al milisegundo.
type RateLimiter struct {
	rdb    *redis.Client
	prefix string
}

func NewRateLimiter(rdb *redis.Client) *RateLimiter {
	return &RateLimiter{rdb: rdb, prefix: "rl:"}
}

// incrWithTTL incrementa y fija el vencimiento solo la primera vez, de forma atómica.
var incrWithTTL = redis.NewScript(`
local n = redis.call("INCR", KEYS[1])
if n == 1 then redis.call("PEXPIRE", KEYS[1], ARGV[1]) end
return {n, redis.call("PTTL", KEYS[1])}
`)

func (l *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	res, err := incrWithTTL.Run(ctx, l.rdb, []string{l.prefix + key}, window.Milliseconds()).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("%w: límite de solicitudes: %v", ports.ErrDependencyUnavailable, err)
	}
	count, ttl := res[0], time.Duration(res[1])*time.Millisecond
	if count > int64(limit) {
		return false, ttl, nil
	}
	return true, 0, nil
}

// SpendGuard es el tope global de gasto diario en IA, en micro-dólares
// (1 USD = 1.000.000). El día se corta en UTC: es un límite de costo nuestro,
// no del usuario.
type SpendGuard struct {
	rdb      *redis.Client
	capMicro int64
}

func NewSpendGuard(rdb *redis.Client, dailyCapUSD float64) *SpendGuard {
	return &SpendGuard{rdb: rdb, capMicro: int64(dailyCapUSD * 1_000_000)}
}

func spendKey(now time.Time) string { return "ai:spend:" + now.UTC().Format("2006-01-02") }

// reserve suma y, si se pasa del tope, deshace la suma en la misma operación.
var reserve = redis.NewScript(`
local total = redis.call("INCRBY", KEYS[1], ARGV[1])
if total == tonumber(ARGV[1]) then redis.call("EXPIRE", KEYS[1], 172800) end
if total > tonumber(ARGV[2]) then
  redis.call("DECRBY", KEYS[1], ARGV[1])
  return 0
end
return 1
`)

// Reserve falla cerrado: si Redis no responde, no se gasta (protege el costo).
func (g *SpendGuard) Reserve(ctx context.Context, microUSD int64, now time.Time) error {
	ok, err := reserve.Run(ctx, g.rdb, []string{spendKey(now)}, microUSD, g.capMicro).Int()
	if err != nil {
		return fmt.Errorf("%w: tope de gasto en IA: %v", ports.ErrDependencyUnavailable, err)
	}
	if ok == 0 {
		return ports.ErrAIBudgetExceeded
	}
	return nil
}

// Release devuelve una reserva cuando la llamada a la IA falló (no se cobró).
func (g *SpendGuard) Release(ctx context.Context, microUSD int64, now time.Time) error {
	if err := g.rdb.DecrBy(ctx, spendKey(now), microUSD).Err(); err != nil {
		return fmt.Errorf("liberando gasto en IA: %w", err)
	}
	return nil
}
