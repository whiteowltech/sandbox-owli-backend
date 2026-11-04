package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

// AIConfig son los parámetros del análisis con IA (desde la configuración).
type AIConfig struct {
	// PrimaryTimeout: si el proveedor principal tarda más, se pasa al respaldo.
	PrimaryTimeout time.Duration
	// FallbackTimeout: límite del respaldo; pasado ese tiempo, se devuelve el crédito.
	FallbackTimeout time.Duration
	// FallbackGramsFactor corrige el sesgo del respaldo (Haiku sobreestima ~39 %; benchmark de nov. 2026).
	FallbackGramsFactor float64
	// CostMicroUSD es el costo estimado por análisis de foto, para el tope diario de gasto.
	CostMicroUSD int64
	// VoiceCostMicroUSD: transcripción (Whisper factura 10 s como mínimo) + extracción en modo texto.
	VoiceCostMicroUSD int64
	// TranscribeTimeout: límite de la transcripción; pasado ese tiempo, se devuelve el crédito.
	TranscribeTimeout time.Duration
	// TextCostMicroUSD: solo la extracción en modo texto (el dispositivo ya transcribió).
	TextCostMicroUSD int64
}

// AnalysisService implementa el registro por foto y por voz (CLAUDE.md, "Pipeline de registro de comidas"):
//  1. cobra 1 crédito (atómico); 2. reserva el gasto en el tope diario;
//  3. pide a la IA solo alimentos y gramos (primario → respaldo, sin cobrar de nuevo);
//     en voz, antes transcribe con Whisper y la extracción es en modo texto;
//  4. si falla o no hay comida, devuelve el crédito; 5. cruza con la base nutricional.
//
// Ni la foto ni el audio se guardan: solo viven en memoria durante el pedido.
type AnalysisService struct {
	credits  ports.CreditStore
	policy   domain.CreditPolicy
	profiles ports.ProfileRepository
	foods    ports.FoodRepository
	spend    ports.AISpendGuard
	primary  ports.FoodRecognizer // nil = IA no configurada
	fallback ports.FoodRecognizer // nil = sin respaldo
	// Voz (opcional; ver WithVoice).
	transcriber  ports.Transcriber
	textPrimary  ports.TextFoodRecognizer
	textFallback ports.TextFoodRecognizer
	cfg          AIConfig
	log          *slog.Logger
	now          func() time.Time
}

func NewAnalysisService(credits ports.CreditStore, policy domain.CreditPolicy, profiles ports.ProfileRepository, foods ports.FoodRepository,
	spend ports.AISpendGuard, primary, fallback ports.FoodRecognizer, cfg AIConfig, log *slog.Logger, now func() time.Time) *AnalysisService {
	return &AnalysisService{credits: credits, policy: policy, profiles: profiles, foods: foods, spend: spend,
		primary: primary, fallback: fallback, cfg: cfg, log: log, now: now}
}

// WithVoice habilita el registro por voz. Sin transcriptor o sin modelo de texto,
// AnalyzeVoice responde ErrAIUnavailable.
func (s *AnalysisService) WithVoice(t ports.Transcriber, primary, fallback ports.TextFoodRecognizer) *AnalysisService {
	s.transcriber, s.textPrimary, s.textFallback = t, primary, fallback
	return s
}

// localDay es el día del usuario: la cuota diaria se cuenta en su zona horaria.
func (s *AnalysisService) localDay(ctx context.Context, userID domain.UserID) (string, error) {
	p, err := s.profiles.Get(ctx, userID)
	if errors.Is(err, ports.ErrNotFound) {
		return "", ports.ErrProfileRequired
	}
	if err != nil {
		return "", fmt.Errorf("leyendo perfil: %w", err)
	}
	return domain.Today(s.now(), p.Timezone)
}

func (s *AnalysisService) Credits(ctx context.Context, userID domain.UserID) (domain.CreditWallet, domain.CreditPolicy, error) {
	day, err := s.localDay(ctx, userID)
	if err != nil {
		return domain.CreditWallet{}, s.policy, err
	}
	w, err := s.credits.Wallet(ctx, userID, day, s.policy.WelcomeCredits)
	return w, s.policy, err
}

func newAnalysisID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *AnalysisService) AnalyzePhoto(ctx context.Context, userID domain.UserID, img ports.Image) (ports.Analysis, error) {
	if s.primary == nil {
		return ports.Analysis{}, fmt.Errorf("%w: IA no configurada", ports.ErrAIUnavailable)
	}
	var fb recognizeFunc
	if s.fallback != nil {
		fb = func(ctx context.Context) (ports.RecognitionResult, error) { return s.fallback.RecognizeImage(ctx, img) }
	}
	return s.analyze(ctx, userID, s.cfg.CostMicroUSD, func(ctx context.Context) (recognized, error) {
		return s.recognize(ctx,
			func(ctx context.Context) (ports.RecognitionResult, error) { return s.primary.RecognizeImage(ctx, img) },
			fb, s.cfg.FallbackGramsFactor)
	})
}

// AnalyzeVoice transcribe el audio y extrae alimentos y gramos del texto.
// Si no se entendió nada (silencio, ruido), no hay comida y no se cobra.
func (s *AnalysisService) AnalyzeVoice(ctx context.Context, userID domain.UserID, audio ports.Audio) (ports.Analysis, error) {
	if s.transcriber == nil || s.textPrimary == nil {
		return ports.Analysis{}, fmt.Errorf("%w: voz no configurada", ports.ErrAIUnavailable)
	}
	return s.analyze(ctx, userID, s.cfg.VoiceCostMicroUSD, func(ctx context.Context) (recognized, error) {
		tctx, cancel := context.WithTimeout(ctx, s.cfg.TranscribeTimeout)
		tr, err := s.transcriber.Transcribe(tctx, audio)
		cancel()
		if err != nil {
			return recognized{}, fmt.Errorf("transcripción: %w", err)
		}
		return s.recognizeTranscript(ctx, tr.Text)
	})
}

// AnalyzeTranscript analiza lo que el dispositivo ya transcribió (reconocimiento de
// voz del navegador o del teléfono): no hay audio ni Whisper, solo la extracción en
// modo texto. Cuesta un crédito igual que la voz, porque la extracción usa IA.
func (s *AnalysisService) AnalyzeTranscript(ctx context.Context, userID domain.UserID, text string) (ports.Analysis, error) {
	if s.textPrimary == nil {
		return ports.Analysis{}, fmt.Errorf("%w: análisis de texto no configurado", ports.ErrAIUnavailable)
	}
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > ports.MaxTranscriptRunes {
		return ports.Analysis{}, ports.ErrInvalidTranscript
	}
	return s.analyze(ctx, userID, s.cfg.TextCostMicroUSD, func(ctx context.Context) (recognized, error) {
		return s.recognizeTranscript(ctx, text)
	})
}

// VoiceAvailability: qué caminos de voz están configurados en el servidor.
func (s *AnalysisService) VoiceAvailability() ports.VoiceAvailability {
	return ports.VoiceAvailability{Transcript: s.textPrimary != nil, Audio: s.transcriber != nil && s.textPrimary != nil}
}

// recognizeTranscript extrae alimentos y gramos de un texto dictado. Un texto vacío
// (silencio, ruido) es "sin comida" y no se cobra.
func (s *AnalysisService) recognizeTranscript(ctx context.Context, text string) (recognized, error) {
	if strings.TrimSpace(text) == "" {
		return recognized{}, nil
	}
	var fb recognizeFunc
	if s.textFallback != nil {
		fb = func(ctx context.Context) (ports.RecognitionResult, error) {
			return s.textFallback.RecognizeText(ctx, text)
		}
	}
	// Sin factor de gramos en el respaldo: la calibración se midió con fotos,
	// y en voz el usuario muchas veces dice la cantidad.
	r, err := s.recognize(ctx,
		func(ctx context.Context) (ports.RecognitionResult, error) {
			return s.textPrimary.RecognizeText(ctx, text)
		},
		fb, 1)
	r.transcript = text
	return r, err
}

type recognizeFunc func(ctx context.Context) (ports.RecognitionResult, error)

type recognized struct {
	res        ports.RecognitionResult
	fallback   bool
	transcript string
}

// analyze es el flujo común: cobra, reserva el gasto, llama a la IA y cruza con la base.
func (s *AnalysisService) analyze(ctx context.Context, userID domain.UserID, cost int64,
	run func(ctx context.Context) (recognized, error)) (ports.Analysis, error) {
	day, err := s.localDay(ctx, userID)
	if err != nil {
		return ports.Analysis{}, err
	}
	id := newAnalysisID()
	_, wallet, err := s.credits.Debit(ctx, userID, day, id, s.policy.WelcomeCredits, s.policy.PickDebitBucket)
	if err != nil {
		return ports.Analysis{}, err // sin créditos: no se llama a la IA
	}

	now := s.now()
	if err := s.spend.Reserve(ctx, cost, now); err != nil {
		s.refund(ctx, userID, id)
		return ports.Analysis{}, fmt.Errorf("%w: %v", ports.ErrAIUnavailable, err)
	}

	r, err := run(ctx)
	if err != nil {
		_ = s.spend.Release(context.WithoutCancel(ctx), cost, now)
		s.refund(ctx, userID, id)
		s.log.Warn("análisis con IA fallido: crédito devuelto", "analysis", id, "err", err)
		return ports.Analysis{}, fmt.Errorf("%w: %v", ports.ErrAIUnavailable, err)
	}

	out := ports.Analysis{ID: id, Provider: r.res.Model, Fallback: r.fallback, Transcript: r.transcript, Wallet: wallet}
	if r.res.Recognition.IsEmpty() {
		// No se cobra si no hay comida (la IA sí se usó: el gasto queda).
		s.refund(ctx, userID, id)
		out.NoFood = true
		out.Wallet, _ = s.credits.Wallet(ctx, userID, day, s.policy.WelcomeCredits)
		return out, nil
	}
	for _, it := range r.res.Recognition.Items {
		item := ports.AnalysisItem{Recognized: it, NeedsReview: it.Confidence < domain.LowConfidenceThreshold}
		if f, ok := s.match(ctx, it); ok {
			item.Food = &f
		} else {
			item.NeedsReview = true
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// recognize prueba el proveedor principal con su límite de tiempo y, si falla
// por un motivo transitorio o por tiempo, el de respaldo. El reintento no cobra.
// gramsFactor corrige el sesgo conocido del respaldo (1 = sin corrección).
func (s *AnalysisService) recognize(ctx context.Context, primary, fallback recognizeFunc, gramsFactor float64) (recognized, error) {
	pctx, cancel := context.WithTimeout(ctx, s.cfg.PrimaryTimeout)
	res, err := primary(pctx)
	cancel()
	if err == nil {
		return recognized{res: res}, res.Recognition.Validate()
	}
	transient := ports.IsRetryable(err) || errors.Is(err, context.DeadlineExceeded)
	if !transient || fallback == nil || ctx.Err() != nil {
		return recognized{}, err
	}
	s.log.Info("IA principal no disponible: usando respaldo", "err", err)
	fctx, fcancel := context.WithTimeout(ctx, s.cfg.FallbackTimeout)
	defer fcancel()
	res, ferr := fallback(fctx)
	if ferr != nil {
		return recognized{fallback: true}, fmt.Errorf("principal: %v; respaldo: %w", err, ferr)
	}
	for i := range res.Recognition.Items {
		g := res.Recognition.Items[i].Grams * gramsFactor
		res.Recognition.Items[i].Grams = math.Max(1, math.Round(g))
	}
	return recognized{res: res, fallback: true}, res.Recognition.Validate()
}

// match busca el alimento en la base con los dos nombres de la IA: el que se muestra
// en español (en voz, lo que dijo el usuario: "bife") y el genérico en inglés
// ("beef steak"). Gana la coincidencia más fuerte; si no hay, el usuario elige a mano.
func (s *AnalysisService) match(ctx context.Context, it domain.RecognizedFood) (domain.Food, bool) {
	f, ok, err := s.foods.Match(ctx, []string{it.DisplayName, it.Name})
	if err != nil {
		s.log.Warn("no se pudo cruzar con la base", "food", it.Name, "err", err)
		return domain.Food{}, false
	}
	return f, ok
}

func (s *AnalysisService) refund(ctx context.Context, userID domain.UserID, id string) {
	// Aunque el pedido se haya cancelado, el reembolso tiene que quedar registrado.
	if err := s.credits.Refund(context.WithoutCancel(ctx), userID, id); err != nil {
		s.log.Error("no se pudo devolver el crédito", "analysis", id, "user", userID, "err", err)
	}
}

var _ ports.AnalysisUseCase = (*AnalysisService)(nil)
