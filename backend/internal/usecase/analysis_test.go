package usecase

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/whiteowltech/owli/backend/internal/adapters/fakeai"
	"github.com/whiteowltech/owli/backend/internal/adapters/memory"
	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

type spendSpy struct {
	reserved, released int64
	full               bool
}

func (s *spendSpy) Reserve(_ context.Context, m int64, _ time.Time) error {
	if s.full {
		return ports.ErrAIBudgetExceeded
	}
	s.reserved += m
	return nil
}

func (s *spendSpy) Release(_ context.Context, m int64, _ time.Time) error {
	s.released += m
	return nil
}

type analysisEnv struct {
	svc     *AnalysisService
	credits *memory.Credits
	spend   *spendSpy
	user    domain.UserID
}

var testPolicy = domain.CreditPolicy{FreeDailyQuota: 1, WelcomeCredits: 15, ProFairUseDaily: 25}

func newAnalysis(t *testing.T, primary, fallback ports.FoodRecognizer) analysisEnv {
	t.Helper()
	svc, store, u := newDiary(t) // catálogo con pechuga de pollo y arroz blanco
	_ = svc
	saveProfile(t, store, u)
	credits := memory.NewCredits()
	spend := &spendSpy{}
	a := NewAnalysisService(credits, testPolicy, store, store.FoodsRepo(), spend, primary, fallback,
		AIConfig{PrimaryTimeout: 50 * time.Millisecond, FallbackTimeout: time.Second, FallbackGramsFactor: 0.71, CostMicroUSD: 3000},
		slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return diaryNow })
	return analysisEnv{svc: a, credits: credits, spend: spend, user: u}
}

func photo(content string) ports.Image {
	return ports.Image{Data: []byte(content), MIMEType: "image/jpeg"}
}

func TestAnalyzePhotoChargesAndMatches(t *testing.T) {
	env := newAnalysis(t, fakeai.Recognizer{Model: "primario"}, fakeai.Recognizer{Model: "respaldo"})
	got, err := env.svc.AnalyzePhoto(context.Background(), env.user, photo("plato"))
	if err != nil {
		t.Fatalf("AnalyzePhoto: %v", err)
	}
	if got.Provider != "primario" || got.Fallback || got.NoFood || len(got.Items) != 3 {
		t.Fatalf("análisis = %+v", got)
	}
	chicken, rice, glaze := got.Items[0], got.Items[1], got.Items[2]
	if chicken.Food == nil || chicken.Food.ID != "pechuga-pollo" || chicken.NeedsReview {
		t.Errorf("pollo = %+v", chicken)
	}
	if rice.Food == nil || rice.Food.ID != "arroz-blanco" {
		t.Errorf("arroz = %+v", rice)
	}
	if glaze.Food != nil || !glaze.NeedsReview {
		t.Errorf("el glaseado no está en la base y tiene confianza baja: se marca para revisar, %+v", glaze)
	}
	// Se cobró la cuota diaria (1) y la bienvenida sigue intacta.
	if got.Wallet.UsedToday != 1 || got.Wallet.WelcomeBalance != 15 {
		t.Errorf("billetera = %+v", got.Wallet)
	}
	if env.spend.reserved != 3000 || env.spend.released != 0 {
		t.Errorf("gasto reservado %d, liberado %d", env.spend.reserved, env.spend.released)
	}
}

func TestFallbackWhenPrimaryFailsOrIsSlow(t *testing.T) {
	for _, content := range []string{"FAIL", "SLOW"} {
		t.Run(content, func(t *testing.T) {
			// El principal falla o tarda; el respaldo recibe otra foto que sí responde.
			primary := fakeai.Recognizer{Model: "primario"}
			env := newAnalysis(t, onlyFor(primary, content), fakeai.Recognizer{Model: "respaldo"})
			got, err := env.svc.AnalyzePhoto(context.Background(), env.user, photo("plato"))
			if err != nil {
				t.Fatalf("AnalyzePhoto: %v", err)
			}
			if !got.Fallback || got.Provider != "respaldo" {
				t.Errorf("debía responder el respaldo: %+v", got)
			}
			if g := got.Items[0].Recognized.Grams; g != 107 { // 150 g × 0,71
				t.Errorf("gramos del respaldo = %v, se esperaba 107 (calibración ×0,71)", g)
			}
			if got.Wallet.UsedToday != 1 {
				t.Error("el reintento con el respaldo no cobra otro crédito")
			}
		})
	}
}

// onlyFor hace que el reconocedor falle o tarde según el contenido, pero que el
// respaldo reciba una imagen "normal" (simula proveedores distintos).
type failing struct {
	ports.FoodRecognizer
	mode string
}

func onlyFor(r ports.FoodRecognizer, mode string) ports.FoodRecognizer { return failing{r, mode} }

func (f failing) RecognizeImage(ctx context.Context, _ ports.Image) (ports.RecognitionResult, error) {
	return f.FoodRecognizer.RecognizeImage(ctx, photo(f.mode))
}

func TestAllProvidersFailRefundsTheCredit(t *testing.T) {
	env := newAnalysis(t, fakeai.Recognizer{}, fakeai.Recognizer{})
	_, err := env.svc.AnalyzePhoto(context.Background(), env.user, photo("FAIL"))
	if !errors.Is(err, ports.ErrAIUnavailable) {
		t.Fatalf("error = %v, se esperaba ErrAIUnavailable", err)
	}
	w, _, _ := env.svc.Credits(context.Background(), env.user)
	if w.UsedToday != 0 || w.WelcomeBalance != 15 {
		t.Errorf("el crédito debía volver: %+v", w)
	}
	if env.spend.released != 3000 {
		t.Errorf("el gasto reservado debía liberarse: %d", env.spend.released)
	}
}

func TestNoFoodIsNotCharged(t *testing.T) {
	env := newAnalysis(t, fakeai.Recognizer{}, nil)
	got, err := env.svc.AnalyzePhoto(context.Background(), env.user, photo("EMPTY"))
	if err != nil || !got.NoFood {
		t.Fatalf("sin comida = %+v, %v", got, err)
	}
	if got.Wallet.UsedToday != 0 {
		t.Errorf("no se cobra si no hay comida: %+v", got.Wallet)
	}
}

func TestOutOfCreditsDoesNotCallTheAI(t *testing.T) {
	calls := 0
	env := newAnalysis(t, counting{&calls}, nil)
	ctx := context.Background()
	for range 16 { // 1 diario + 15 de bienvenida
		if _, err := env.svc.AnalyzePhoto(ctx, env.user, photo("plato")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := env.svc.AnalyzePhoto(ctx, env.user, photo("plato")); !errors.Is(err, domain.ErrInsufficientCredits) {
		t.Fatalf("sin créditos = %v", err)
	}
	if calls != 16 {
		t.Errorf("la IA se llamó %d veces, se esperaba 16 (sin créditos no se llama)", calls)
	}
}

type counting struct{ n *int }

func (c counting) RecognizeImage(ctx context.Context, img ports.Image) (ports.RecognitionResult, error) {
	*c.n++
	return fakeai.Recognizer{}.RecognizeImage(ctx, img)
}

func TestBudgetExceededRefundsAndSkipsTheAI(t *testing.T) {
	calls := 0
	env := newAnalysis(t, counting{&calls}, nil)
	env.spend.full = true
	if _, err := env.svc.AnalyzePhoto(context.Background(), env.user, photo("plato")); !errors.Is(err, ports.ErrAIUnavailable) {
		t.Fatalf("tope de gasto = %v", err)
	}
	if calls != 0 {
		t.Error("con el tope alcanzado no se llama a la IA")
	}
	if w, _, _ := env.svc.Credits(context.Background(), env.user); w.UsedToday != 0 {
		t.Error("el crédito debía volver")
	}
}

func TestAnalysisNeedsProfileAndConfiguredAI(t *testing.T) {
	env := newAnalysis(t, nil, nil)
	if _, err := env.svc.AnalyzePhoto(context.Background(), env.user, photo("plato")); !errors.Is(err, ports.ErrAIUnavailable) {
		t.Errorf("sin IA configurada = %v", err)
	}
}

// --- Voz ---

const voiceCost = 1000

func newVoice(t *testing.T, primary, fallback ports.TextFoodRecognizer) analysisEnv {
	t.Helper()
	env := newAnalysis(t, fakeai.Recognizer{}, nil)
	env.svc.cfg.VoiceCostMicroUSD = voiceCost
	env.svc.cfg.TranscribeTimeout = time.Second
	env.svc.WithVoice(fakeai.Transcriber{}, primary, fallback)
	return env
}

func voice(content string) ports.Audio {
	return ports.Audio{Data: []byte(content), MIMEType: "audio/webm"}
}

// textDown simula un modelo de texto caído.
type textDown struct{}

func (textDown) RecognizeText(context.Context, string) (ports.RecognitionResult, error) {
	return ports.RecognitionResult{}, ports.ErrProviderUnavailable
}

func TestAnalyzeVoiceTranscribesChargesAndMatches(t *testing.T) {
	env := newVoice(t, fakeai.Recognizer{Model: "texto"}, nil)
	got, err := env.svc.AnalyzeVoice(context.Background(), env.user, voice("audio"))
	if err != nil {
		t.Fatalf("AnalyzeVoice: %v", err)
	}
	if got.Transcript != fakeai.Phrase {
		t.Errorf("la transcripción se devuelve para mostrarla: %q", got.Transcript)
	}
	if got.Provider != "texto" || got.NoFood || len(got.Items) != 3 {
		t.Fatalf("análisis = %+v", got)
	}
	if got.Items[0].Food == nil || got.Items[0].Food.ID != "pechuga-pollo" || got.Items[2].Food != nil {
		t.Errorf("cruce con la base = %+v", got.Items)
	}
	if got.Wallet.UsedToday != 1 || env.spend.reserved != voiceCost {
		t.Errorf("cobra 1 análisis y reserva el costo de voz: %+v, %d", got.Wallet, env.spend.reserved)
	}
}

func TestVoiceFallbackKeepsTheGrams(t *testing.T) {
	env := newVoice(t, textDown{}, fakeai.Recognizer{Model: "respaldo"})
	got, err := env.svc.AnalyzeVoice(context.Background(), env.user, voice("audio"))
	if err != nil {
		t.Fatalf("AnalyzeVoice: %v", err)
	}
	if !got.Fallback || got.Provider != "respaldo" {
		t.Fatalf("debía usar el respaldo: %+v", got)
	}
	// El factor 0,71 se midió con fotos: en voz no se aplica.
	if g := got.Items[0].Recognized.Grams; g != 150 {
		t.Errorf("gramos del respaldo = %v, se esperaban 150 sin corregir", g)
	}
	if got.Wallet.UsedToday != 1 {
		t.Errorf("el respaldo no cobra de nuevo: %+v", got.Wallet)
	}
}

func TestVoiceSilenceIsNotCharged(t *testing.T) {
	env := newVoice(t, fakeai.Recognizer{}, nil)
	got, err := env.svc.AnalyzeVoice(context.Background(), env.user, voice("SILENCE"))
	if err != nil || !got.NoFood {
		t.Fatalf("silencio = %+v, %v", got, err)
	}
	if got.Wallet.UsedToday != 0 || got.Wallet.WelcomeBalance != 15 {
		t.Errorf("no se cobra si no se entendió nada: %+v", got.Wallet)
	}
}

func TestVoiceFailuresRefund(t *testing.T) {
	for name, env := range map[string]analysisEnv{
		"transcripción caída": newVoice(t, fakeai.Recognizer{}, nil),
		"texto caído":         newVoice(t, textDown{}, textDown{}),
	} {
		audio := voice("audio")
		if name == "transcripción caída" {
			audio = voice("FAIL")
		}
		_, err := env.svc.AnalyzeVoice(context.Background(), env.user, audio)
		if !errors.Is(err, ports.ErrAIUnavailable) {
			t.Fatalf("%s: error = %v", name, err)
		}
		w, _, _ := env.svc.Credits(context.Background(), env.user)
		if w.UsedToday != 0 || w.WelcomeBalance != 15 || env.spend.released != voiceCost {
			t.Errorf("%s: el crédito y el gasto debían volver: %+v, liberado %d", name, w, env.spend.released)
		}
	}
}

func TestVoiceNotConfigured(t *testing.T) {
	env := newAnalysis(t, fakeai.Recognizer{}, nil)
	if _, err := env.svc.AnalyzeVoice(context.Background(), env.user, voice("audio")); !errors.Is(err, ports.ErrAIUnavailable) {
		t.Errorf("sin voz configurada: %v", err)
	}
	w, _, _ := env.svc.Credits(context.Background(), env.user)
	if w.UsedToday != 0 {
		t.Errorf("sin voz configurada no se cobra: %+v", w)
	}
}

// --- Texto transcrito en el dispositivo ---

func TestAnalyzeTranscript(t *testing.T) {
	env := newVoice(t, fakeai.Recognizer{Model: "texto"}, nil)
	env.svc.cfg.TextCostMicroUSD = 500
	ctx := context.Background()

	got, err := env.svc.AnalyzeTranscript(ctx, env.user, "  dos pechugas de pollo con arroz  ")
	if err != nil {
		t.Fatal(err)
	}
	if got.Transcript != "dos pechugas de pollo con arroz" || len(got.Items) != 3 || got.Wallet.UsedToday != 1 {
		t.Fatalf("análisis = %+v", got)
	}
	if env.spend.reserved != 500 {
		t.Errorf("reserva solo el costo de la extracción: %d", env.spend.reserved)
	}

	// Sin comida en lo dicho: no se cobra.
	got, err = env.svc.AnalyzeTranscript(ctx, env.user, "NADA, solo agua")
	if err != nil || !got.NoFood || got.Wallet.UsedToday != 1 {
		t.Errorf("sin comida = %+v, %v", got, err)
	}

	// Vacío o demasiado largo: error de validación, sin cobrar ni llamar a la IA.
	for _, bad := range []string{"   ", strings.Repeat("a", ports.MaxTranscriptRunes+1)} {
		if _, err := env.svc.AnalyzeTranscript(ctx, env.user, bad); !errors.Is(err, ports.ErrInvalidTranscript) {
			t.Errorf("texto inválido (%d): %v", len(bad), err)
		}
	}
	w, _, _ := env.svc.Credits(ctx, env.user)
	if w.UsedToday != 1 {
		t.Errorf("solo el primero cobró: %+v", w)
	}
}

func TestVoiceAvailability(t *testing.T) {
	env := newAnalysis(t, fakeai.Recognizer{}, nil)
	if v := env.svc.VoiceAvailability(); v.Transcript || v.Audio {
		t.Errorf("sin voz configurada: %+v", v)
	}
	env.svc.WithVoice(nil, fakeai.Recognizer{}, nil) // sin Groq: solo texto del dispositivo
	if v := env.svc.VoiceAvailability(); !v.Transcript || v.Audio {
		t.Errorf("sin Whisper: %+v", v)
	}
	if _, err := env.svc.AnalyzeVoice(context.Background(), env.user, voice("audio")); !errors.Is(err, ports.ErrAIUnavailable) {
		t.Errorf("sin Whisper el audio no se acepta: %v", err)
	}
}
