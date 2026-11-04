package httpapi

import (
	"context"
	"log/slog"
	"math"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

type ctxKey struct{}

// clientIP es la IP del cliente sin el puerto (que cambia en cada conexión y
// haría inútil un límite por IP). middleware.RealIP ya resolvió X-Forwarded-For.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// userID devuelve el usuario autenticado. Es la única fuente de identidad:
// nunca se toma un user_id del body ni de la URL.
func userID(ctx context.Context) domain.UserID {
	id, _ := ctx.Value(ctxKey{}).(domain.UserID)
	return id
}

// authenticate exige "Authorization: Bearer <access token>".
func authenticate(v ports.AuthVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				writeError(w, nil, apiError{http.StatusUnauthorized, "unauthenticated", "Falta el token de acceso."})
				return
			}
			id, err := v.Verify(r.Context(), token)
			if err != nil {
				writeError(w, nil, apiError{http.StatusUnauthorized, "invalid_token", "El token de acceso no es válido o venció."})
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
		})
	}
}

// rateLimit falla abierto: si Redis no responde, se deja pasar y se registra.
// Un corte de Redis no debe dejar a los usuarios afuera; el tope de gasto en IA
// (que sí falla cerrado) protege el costo.
func rateLimit(l ports.RateLimiter, log *slog.Logger, limit int, window time.Duration, key func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if l == nil || limit <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			ok, retry, err := l.Allow(r.Context(), key(r), limit, window)
			if err != nil {
				log.Warn("límite de solicitudes no disponible: se deja pasar", "err", err)
				next.ServeHTTP(w, r)
				return
			}
			if !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
				writeError(w, nil, apiError{http.StatusTooManyRequests, "rate_limited", "Demasiadas solicitudes. Prueba de nuevo en un rato."})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// cors habilita a la app (ionic serve en desarrollo, Capacitor en el teléfono).
// La sesión viaja en el header Authorization, no en cookies.
func cors(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && slices.Contains(allowed, origin) {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Add("Vary", "Origin")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				h.Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
