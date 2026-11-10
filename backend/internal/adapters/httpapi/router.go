// Package httpapi es el adapter HTTP (Chi). Es la única capa que conoce
// net/http y la única que traduce errores de dominio a códigos de estado.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/whiteowltech/owli/backend/internal/ports"
)

// Limits son los topes de solicitudes por IP o por usuario (contra bots y abuso).
type Limits struct {
	AnonymousPerIPHour int // cuentas anónimas nuevas por IP y hora
	SignInPerIPHour    int // inicios de sesión y renovaciones por IP y hora
	UserPerMinute      int // pedidos autenticados por usuario y minuto
	AIPerUserHour      int // análisis con IA por usuario y hora (aparte de los créditos)
}

// DefaultLimits son holgados para personas y cortos para scripts.
var DefaultLimits = Limits{AnonymousPerIPHour: 10, SignInPerIPHour: 60, UserPerMinute: 120, AIPerUserHour: 30}

// Deps son las dependencias del router (inyectadas desde cmd/api).
type Deps struct {
	Logger         *slog.Logger
	Auth           ports.AuthUseCase
	Profiles       ports.ProfileUseCase
	Diary          ports.DiaryUseCase
	Analysis       ports.AnalysisUseCase
	Suggestions    ports.SuggestionUseCase
	Verifier       ports.AuthVerifier
	Limiter        ports.RateLimiter
	Limits         Limits
	AllowedOrigins []string // CORS: orígenes de la app (dev y Capacitor)
}

// NewRouter construye el router con los middlewares comunes.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors(d.AllowedOrigins))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	if d.Auth == nil {
		return r // modo mínimo (sin base de datos): solo /healthz
	}
	h := &handlers{auth: d.Auth, profiles: d.Profiles, log: d.Logger}
	byIP := func(prefix string, limit int, window time.Duration) func(http.Handler) http.Handler {
		return rateLimit(d.Limiter, d.Logger, limit, window, func(r *http.Request) string { return prefix + ":ip:" + clientIP(r) })
	}

	r.Route("/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.With(byIP("anon", d.Limits.AnonymousPerIPHour, time.Hour)).Post("/anonymous", h.createAnonymous)
			r.Group(func(r chi.Router) {
				r.Use(byIP("signin", d.Limits.SignInPerIPHour, time.Hour))
				r.Post("/refresh", h.refresh)
				r.Post("/logout", h.logout)
				r.Post("/signin/{provider}", h.signIn)
			})
			r.With(authenticate(d.Verifier), perUser(d)).Post("/link/{provider}", h.link)
		})
		r.Group(func(r chi.Router) {
			r.Use(authenticate(d.Verifier), perUser(d))
			r.Get("/me/profile", h.getProfile)
			r.Put("/me/profile", h.putProfile)
			r.Delete("/me", h.deleteAccount)
			if d.Diary != nil {
				dh := &diaryHandlers{diary: d.Diary, h: h}
				r.Get("/foods", dh.searchFoods)
				r.Post("/me/entries", dh.logEntry)
				r.Post("/me/recipes", dh.logRecipe)
				r.Delete("/me/entries/{id}", dh.deleteEntry)
				r.Get("/me/day", dh.day)
				r.Get("/me/week", dh.week)
			}
			if d.Suggestions != nil {
				sh := &suggestionHandlers{uc: d.Suggestions, h: h}
				r.Get("/me/suggestions", sh.list)
			}
			if d.Analysis != nil {
				ah := &analysisHandlers{analysis: d.Analysis, h: h}
				r.Get("/me/credits", ah.credits)
				// Límite propio de los endpoints de IA (CLAUDE.md), independiente de los créditos.
				// Foto y voz comparten el contador.
				ai := r.With(rateLimit(d.Limiter, d.Logger, d.Limits.AIPerUserHour, time.Hour, func(r *http.Request) string {
					return "ai:user:" + string(userID(r.Context()))
				}))
				ai.Post("/me/analyses/photo", ah.analyzePhoto)
				ai.Post("/me/analyses/voice", ah.analyzeVoice)
				ai.Post("/me/analyses/text", ah.analyzeText)
			}
		})
	})
	return r
}

func perUser(d Deps) func(http.Handler) http.Handler {
	return rateLimit(d.Limiter, d.Logger, d.Limits.UserPerMinute, time.Minute, func(r *http.Request) string {
		return "user:" + string(userID(r.Context()))
	})
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			logger.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
