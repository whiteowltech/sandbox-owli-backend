package ports

import (
	"context"
	"errors"

	"github.com/whiteowltech/owli/backend/internal/domain"
)

// ErrAIUnavailable: ningún proveedor de IA respondió (o se alcanzó el tope de
// gasto). El crédito ya se devolvió: el usuario no pierde nada.
var ErrAIUnavailable = errors.New("el análisis con IA no está disponible ahora")

// PickBucket decide de qué bucket sale un consumo a partir del saldo. Es una
// regla de dominio (domain.CreditPolicy.PickDebitBucket) que el almacén aplica
// dentro de su transacción.
type PickBucket func(domain.CreditWallet) (domain.CreditBucket, error)

// CreditStore es el libro de créditos (append-only). Todas las operaciones son
// atómicas, bloquean la fila del usuario y son idempotentes por referencia.
type CreditStore interface {
	// Wallet devuelve el saldo, dando primero los créditos de bienvenida si
	// todavía no se dieron (una sola vez por usuario).
	Wallet(ctx context.Context, userID domain.UserID, localDay string, welcome int) (domain.CreditWallet, error)
	// Debit consume 1 crédito del bucket que elija pick. Devuelve el bucket y
	// el saldo ya descontado. Los errores de pick (sin créditos) se propagan.
	Debit(ctx context.Context, userID domain.UserID, localDay, reference string, welcome int, pick PickBucket) (domain.CreditBucket, domain.CreditWallet, error)
	// Refund devuelve el crédito del consumo con esa referencia al mismo bucket.
	// Es idempotente; si no hubo consumo, no hace nada.
	Refund(ctx context.Context, userID domain.UserID, reference string) error
}

// AnalysisItem es un alimento detectado, ya cruzado con la base nutricional.
type AnalysisItem struct {
	Recognized  domain.RecognizedFood
	Food        *domain.Food // nil si no se encontró en la base: el usuario lo elige a mano
	NeedsReview bool         // confianza baja o sin coincidencia: se resalta (solo ayuda visual)
}

// Analysis es el borrador (de una foto o de un audio) que el usuario confirma o
// corrige antes de guardar.
type Analysis struct {
	ID    string
	Items []AnalysisItem
	// Transcript es lo que se entendió del audio (vacío en las fotos): se muestra en
	// la confirmación para que el usuario vea de dónde salió cada alimento.
	Transcript string
	Provider   string // modelo que respondió (primario o respaldo)
	Fallback   bool
	// NoFood: no se detectó comida; el crédito se devolvió.
	NoFood bool
	Wallet domain.CreditWallet
}

// AnalysisUseCase: análisis de fotos y audios con IA (implementado en usecase.AnalysisService).
type AnalysisUseCase interface {
	AnalyzePhoto(ctx context.Context, userID domain.UserID, img Image) (Analysis, error)
	AnalyzeVoice(ctx context.Context, userID domain.UserID, audio Audio) (Analysis, error)
	// AnalyzeTranscript: el dispositivo ya pasó la voz a texto (gratis); solo se extraen los alimentos.
	AnalyzeTranscript(ctx context.Context, userID domain.UserID, text string) (Analysis, error)
	VoiceAvailability() VoiceAvailability
	Credits(ctx context.Context, userID domain.UserID) (domain.CreditWallet, domain.CreditPolicy, error)
}

// MaxTranscriptRunes acota lo dictado: una comida se describe en pocas frases.
const MaxTranscriptRunes = 500

// ErrInvalidTranscript: texto vacío o demasiado largo.
var ErrInvalidTranscript = errors.New("la transcripción está vacía o es demasiado larga")

// VoiceAvailability dice qué caminos de voz acepta el servidor. La app transcribe en
// el dispositivo cuando puede (Transcript); si no, sube el audio (Audio, Whisper en Groq).
type VoiceAvailability struct {
	Transcript bool
	Audio      bool
}
