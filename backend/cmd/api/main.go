// Command api arranca el servidor HTTP de Owli. Aquí, y solo aquí, se cablean
// las dependencias (inyección explícita, sin singletons globales).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // zonas horarias embebidas: la imagen distroless no depende del sistema

	"github.com/whiteowltech/owli/backend/data"
	"github.com/whiteowltech/owli/backend/internal/adapters/anthropic"
	"github.com/whiteowltech/owli/backend/internal/adapters/fakeai"
	"github.com/whiteowltech/owli/backend/internal/adapters/gemini"
	"github.com/whiteowltech/owli/backend/internal/adapters/groq"
	"github.com/whiteowltech/owli/backend/internal/adapters/httpapi"
	"github.com/whiteowltech/owli/backend/internal/adapters/jwtauth"
	"github.com/whiteowltech/owli/backend/internal/adapters/oidc"
	"github.com/whiteowltech/owli/backend/internal/adapters/postgres"
	"github.com/whiteowltech/owli/backend/internal/adapters/redisx"
	"github.com/whiteowltech/owli/backend/internal/catalog"
	"github.com/whiteowltech/owli/backend/internal/envfile"
	"github.com/whiteowltech/owli/backend/internal/ports"
	"github.com/whiteowltech/owli/backend/internal/usecase"
	"github.com/whiteowltech/owli/backend/migrations"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("el servidor terminó con error", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	if err := envfile.Load(".env"); err != nil {
		return fmt.Errorf("leyendo .env: %w", err)
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool, migrations.FS); err != nil {
		return err
	}
	rdb, err := redisx.Connect(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer rdb.Close()

	tokens, err := jwtauth.New(cfg.JWTSecret, cfg.JWTSecretPrevious, jwtauth.DefaultTTL, time.Now)
	if err != nil {
		return err
	}
	verifiers, err := identityVerifiers(cfg, logger)
	if err != nil {
		return err
	}
	store := postgres.NewStore(pool)
	foods, err := catalog.Parse(data.FoodsSeed)
	if err != nil {
		return err
	}
	if imported, err := store.SyncCatalog(ctx, foods, data.FoodsSeedVersion()); err != nil {
		return err
	} else if imported {
		logger.Info("catálogo de alimentos actualizado", "alimentos", len(foods), "version", data.FoodsSeedVersion())
	}

	ai, err := aiProviders(cfg, logger)
	if err != nil {
		return err
	}
	dishes, err := catalog.ParseDishes(data.Dishes, foods)
	if err != nil {
		return err
	}
	diary := usecase.NewDiaryService(store.Foods(), store.Entries(), store, time.Now)
	handler := httpapi.NewRouter(httpapi.Deps{
		Logger:      logger,
		Auth:        usecase.NewAuthService(store, store, tokens, verifiers, usecase.DefaultRefreshTTL, time.Now),
		Profiles:    usecase.NewProfileService(store),
		Diary:       diary,
		Suggestions: usecase.NewSuggestionService(diary, store, dishes, time.Now),
		Analysis: usecase.NewAnalysisService(store.Credits(), cfg.Credits, store, store.Foods(),
			redisx.NewSpendGuard(rdb, cfg.AIDailyBudgetUSD), ai.image, ai.imageFallback, cfg.AI, logger, time.Now).
			WithVoice(ai.transcriber, ai.text, ai.textFallback),
		Verifier:       tokens,
		Limiter:        redisx.NewRateLimiter(rdb),
		Limits:         cfg.Limits,
		AllowedOrigins: cfg.AllowedOrigins,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("servidor escuchando", "port", cfg.Port, "proveedores", len(verifiers))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("apagando servidor")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// providers son los clientes de IA. Un campo nil = esa función no está disponible
// (el análisis responde "no disponible" y el registro manual sigue funcionando).
type providers struct {
	image, imageFallback ports.FoodRecognizer
	text, textFallback   ports.TextFoodRecognizer
	transcriber          ports.Transcriber
}

// aiProviders arma la IA: Gemini (principal, visión y texto), Claude Haiku
// (respaldo por disponibilidad) y Whisper en Groq (transcripción de voz).
func aiProviders(cfg config, logger *slog.Logger) (providers, error) {
	var p providers
	if cfg.AIFake {
		logger.Warn("IA simulada (AI_FAKE=1): solo para desarrollo y pruebas")
		fake, fakeFallback := fakeai.Recognizer{Model: "fake-primary"}, fakeai.Recognizer{Model: "fake-fallback"}
		return providers{image: fake, imageFallback: fakeFallback, text: fake, textFallback: fakeFallback,
			transcriber: fakeai.Transcriber{}}, nil
	}
	// Se asigna a la interfaz solo si el cliente existe: un puntero nil dentro de una
	// interfaz no es nil y el servicio creería que hay IA.
	if cfg.GeminiAPIKey != "" {
		g, err := gemini.New(gemini.Config{APIKey: cfg.GeminiAPIKey, Model: cfg.GeminiModel})
		if err != nil {
			return providers{}, err
		}
		p.image, p.text = g, g
	} else {
		logger.Warn("GEMINI_API_KEY vacía: el análisis de fotos y voz no está disponible")
	}
	if cfg.AnthropicAPIKey != "" {
		a, err := anthropic.New(anthropic.Config{APIKey: cfg.AnthropicAPIKey, Model: cfg.AnthropicModel})
		if err != nil {
			return providers{}, err
		}
		p.imageFallback, p.textFallback = a, a
	}
	if cfg.GroqAPIKey != "" {
		g, err := groq.New(groq.Config{APIKey: cfg.GroqAPIKey, Model: cfg.GroqModel, Prompt: groq.DefaultPrompt})
		if err != nil {
			return providers{}, err
		}
		p.transcriber = g
	} else {
		logger.Warn("GROQ_API_KEY vacía: el registro por voz no está disponible")
	}
	return p, nil
}

// identityVerifiers arma Apple y Google solo si tienen client IDs configurados.
func identityVerifiers(cfg config, logger *slog.Logger) (map[ports.IdentityProvider]ports.IdentityVerifier, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	out := map[ports.IdentityProvider]ports.IdentityVerifier{}
	for _, c := range []oidc.Config{oidc.Apple(cfg.AppleClientIDs), oidc.Google(cfg.GoogleClientIDs)} {
		if len(c.Audiences) == 0 {
			logger.Warn("proveedor de identidad sin configurar", "provider", c.Provider)
			continue
		}
		v, err := oidc.New(c, client, time.Now)
		if err != nil {
			return nil, fmt.Errorf("configurando %s: %w", c.Provider, err)
		}
		out[c.Provider] = v
	}
	return out, nil
}
