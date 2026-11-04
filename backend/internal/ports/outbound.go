// Package ports define las interfaces entre el dominio y el mundo exterior.
package ports

import (
	"context"
	"errors"

	"github.com/whiteowltech/owli/backend/internal/domain"
)

// Errores transitorios de un proveedor de IA. Los adapters los envuelven con
// %w; el llamador puede reintentar con backoff o pasar al proveedor de respaldo.
var (
	ErrProviderRateLimited = errors.New("límite de tasa del proveedor alcanzado")
	ErrProviderUnavailable = errors.New("proveedor saturado o no disponible")
)

// IsRetryable indica si err es transitorio.
func IsRetryable(err error) bool {
	return errors.Is(err, ErrProviderRateLimited) || errors.Is(err, ErrProviderUnavailable)
}

// Image es una imagen ya comprimida por el cliente.
type Image struct {
	Data     []byte
	MIMEType string // image/jpeg, image/png, image/webp
}

// TokenUsage es el consumo reportado por el proveedor de IA, para costos.
type TokenUsage struct {
	InputTokens    int
	OutputTokens   int // incluye tokens de razonamiento: se facturan como salida
	ThinkingTokens int // subconjunto de OutputTokens, informativo
}

// RecognitionResult agrupa el reconocimiento con metadatos del proveedor.
type RecognitionResult struct {
	Recognition domain.FoodRecognition
	Model       string
	Usage       TokenUsage
}

// FoodRecognizer identifica alimentos y gramos en una imagen.
// Implementación: adapters/gemini.
type FoodRecognizer interface {
	RecognizeImage(ctx context.Context, img Image) (RecognitionResult, error)
}

// Audio es una grabación de voz del usuario. Como las fotos, no se guarda.
type Audio struct {
	Data     []byte
	MIMEType string // audio/webm, audio/ogg, audio/mp4, audio/mpeg, audio/wav
}

// Transcript es el texto reconocido en un audio.
type Transcript struct {
	Text    string
	Seconds float64 // duración facturable informada por el proveedor
	Model   string
}

// Transcriber pasa la voz a texto. Implementación: adapters/groq (Whisper).
type Transcriber interface {
	Transcribe(ctx context.Context, audio Audio) (Transcript, error)
}

// TextFoodRecognizer identifica alimentos y gramos en lo que el usuario dijo.
// Mismo contrato que FoodRecognizer: la IA no calcula macros.
// Implementaciones: adapters/gemini (principal) y adapters/anthropic (respaldo).
type TextFoodRecognizer interface {
	RecognizeText(ctx context.Context, text string) (RecognitionResult, error)
}
