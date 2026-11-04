package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/whiteowltech/owli/backend/internal/adapters/httpapi"
	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/usecase"
)

// config se lee solo de variables de entorno. Los secretos nunca van en el repo:
// en producción viven en las variables de Railway.
type config struct {
	Port            string
	ShutdownTimeout time.Duration
	Credits         domain.CreditPolicy

	DatabaseURL       string
	RedisURL          string
	JWTSecret         string
	JWTSecretPrevious string // solo durante una rotación del secreto
	AppleClientIDs    []string
	GoogleClientIDs   []string
	AllowedOrigins    []string
	Limits            httpapi.Limits
	// AIDailyBudgetUSD es el tope global de gasto en IA por día (red de seguridad contra bots).
	AIDailyBudgetUSD float64

	AppEnv          string // development | production
	GeminiAPIKey    string // tier de pago siempre (datos de salud)
	GeminiModel     string
	AnthropicAPIKey string // respaldo solo por disponibilidad
	AnthropicModel  string
	GroqAPIKey      string // Whisper (transcripción de voz)
	GroqModel       string
	// AIFake usa el reconocedor simulado (pruebas). Prohibido en producción.
	AIFake bool
	AI     usecase.AIConfig
}

// defaultOrigins: la app en desarrollo (ng serve) y dentro de Capacitor.
var defaultOrigins = []string{"http://localhost:8100", "capacitor://localhost", "http://localhost", "ionic://localhost"}

func loadConfig() (config, error) {
	cfg := config{
		Port:              getEnv("PORT", "8080"),
		ShutdownTimeout:   15 * time.Second,
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		RedisURL:          os.Getenv("REDIS_URL"),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		JWTSecretPrevious: os.Getenv("JWT_SECRET_PREVIOUS"),
		AppleClientIDs:    getList("APPLE_CLIENT_IDS", nil),
		GoogleClientIDs:   getList("GOOGLE_CLIENT_IDS", nil),
		AllowedOrigins:    getList("CORS_ALLOWED_ORIGINS", defaultOrigins),
		Limits:            httpapi.DefaultLimits,
		AppEnv:            getEnv("APP_ENV", "development"),
		GeminiAPIKey:      os.Getenv("GEMINI_API_KEY"),
		GeminiModel:       getEnv("GEMINI_MODEL", "gemini-3.1-flash-lite"),
		AnthropicAPIKey:   os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicModel:    getEnv("ANTHROPIC_MODEL", "claude-haiku-5-5"),
		GroqAPIKey:        os.Getenv("GROQ_API_KEY"),
		GroqModel:         getEnv("GROQ_MODEL", "whisper-large-v3-turbo"),
		AIFake:            os.Getenv("AI_FAKE") == "1",
	}

	var err error
	ints := []struct {
		key string
		dst *int
		def int
	}{
		{"CREDITS_FREE_DAILY_QUOTA", &cfg.Credits.FreeDailyQuota, 1},
		{"CREDITS_WELCOME", &cfg.Credits.WelcomeCredits, 15},
		{"CREDITS_PRO_FAIR_USE_DAILY", &cfg.Credits.ProFairUseDaily, 25},
		{"RATE_LIMIT_ANON_PER_IP_HOUR", &cfg.Limits.AnonymousPerIPHour, httpapi.DefaultLimits.AnonymousPerIPHour},
		{"RATE_LIMIT_SIGNIN_PER_IP_HOUR", &cfg.Limits.SignInPerIPHour, httpapi.DefaultLimits.SignInPerIPHour},
		{"RATE_LIMIT_USER_PER_MINUTE", &cfg.Limits.UserPerMinute, httpapi.DefaultLimits.UserPerMinute},
		{"RATE_LIMIT_AI_PER_USER_HOUR", &cfg.Limits.AIPerUserHour, httpapi.DefaultLimits.AIPerUserHour},
	}
	for _, v := range ints {
		if *v.dst, err = getEnvInt(v.key, v.def); err != nil {
			return config{}, err
		}
	}
	if err := cfg.Credits.Validate(); err != nil {
		return config{}, err
	}
	if cfg.AIDailyBudgetUSD, err = getEnvFloat("AI_DAILY_BUDGET_USD", 20); err != nil {
		return config{}, err
	}
	if cfg.AIDailyBudgetUSD <= 0 {
		return config{}, errors.New("AI_DAILY_BUDGET_USD debe ser mayor que 0")
	}
	if cfg.AIFake && cfg.AppEnv == "production" {
		return config{}, errors.New("AI_FAKE no se puede usar con APP_ENV=production")
	}
	var primaryMs, fallbackMs, cost int
	if primaryMs, err = getEnvInt("AI_PRIMARY_TIMEOUT_MS", 15000); err != nil {
		return config{}, err
	}
	if fallbackMs, err = getEnvInt("AI_FALLBACK_TIMEOUT_MS", 20000); err != nil {
		return config{}, err
	}
	if cost, err = getEnvInt("AI_COST_MICRO_USD", 3000); err != nil { // ≈ $0.003 por análisis
		return config{}, err
	}
	factor, err := getEnvFloat("AI_FALLBACK_GRAMS_FACTOR", 0.71) // calibración de Haiku (benchmark nov. 2026)
	if err != nil {
		return config{}, err
	}
	var voiceCost, transcribeMs int
	// Whisper: $0.04/h con 10 s mínimos (≈ $0.0001) + Gemini en modo texto (≈ $0.0003); redondeado hacia arriba.
	if voiceCost, err = getEnvInt("AI_VOICE_COST_MICRO_USD", 1000); err != nil {
		return config{}, err
	}
	if transcribeMs, err = getEnvInt("AI_TRANSCRIBE_TIMEOUT_MS", 15000); err != nil {
		return config{}, err
	}
	// Texto ya transcrito en el dispositivo: solo Gemini en modo texto (≈ $0.0003).
	textCost, err := getEnvInt("AI_TEXT_COST_MICRO_USD", 500)
	if err != nil {
		return config{}, err
	}
	cfg.AI = usecase.AIConfig{PrimaryTimeout: time.Duration(primaryMs) * time.Millisecond,
		FallbackTimeout: time.Duration(fallbackMs) * time.Millisecond, FallbackGramsFactor: factor, CostMicroUSD: int64(cost),
		VoiceCostMicroUSD: int64(voiceCost), TranscribeTimeout: time.Duration(transcribeMs) * time.Millisecond,
		TextCostMicroUSD: int64(textCost)}

	var missing []string
	for key, val := range map[string]string{"DATABASE_URL": cfg.DatabaseURL, "REDIS_URL": cfg.RedisURL, "JWT_SECRET": cfg.JWTSecret} {
		if val == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return config{}, fmt.Errorf("faltan variables de entorno: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("variable %s debe ser un entero: %w", key, err)
	}
	return n, nil
}

func getEnvFloat(key string, fallback float64) (float64, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("variable %s debe ser un número: %w", key, err)
	}
	return f, nil
}

// getList lee una lista separada por comas.
func getList(key string, fallback []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
