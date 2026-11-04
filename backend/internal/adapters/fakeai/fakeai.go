// Package fakeai es un reconocedor de comida simulado, determinista y gratuito,
// para desarrollo y pruebas de punta a punta. NUNCA se usa en producción:
// cmd/api se niega a arrancar con AI_FAKE en APP_ENV=production.
//
// Responde según el contenido de la imagen, para poder probar cada caso:
//   - contiene "FAIL"  → proveedor no disponible (prueba el reembolso)
//   - contiene "SLOW"  → no responde hasta que vence el plazo (prueba el respaldo)
//   - contiene "EMPTY" → no hay comida (no se cobra)
//   - cualquier otra   → pollo, arroz y un glaseado que no está en la base
//
// Para la voz, Transcriber responde según el audio ("FAIL" → no disponible,
// "SILENCE" → transcripción vacía; cualquier otro → una frase fija) y
// RecognizeText según el texto ("FAIL", "SLOW", "NADA" → sin comida).
// Así funciona también con el micrófono simulado del navegador en los e2e.
package fakeai

import (
	"bytes"
	"context"
	"fmt"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

// Recognizer implementa ports.FoodRecognizer.
type Recognizer struct {
	Model string
}

func (r Recognizer) RecognizeImage(ctx context.Context, img ports.Image) (ports.RecognitionResult, error) {
	return r.respond(ctx, img.Data, "EMPTY")
}

// RecognizeText responde igual que con una foto; "NADA" significa sin comida.
func (r Recognizer) RecognizeText(ctx context.Context, text string) (ports.RecognitionResult, error) {
	return r.respond(ctx, []byte(text), "NADA")
}

func (r Recognizer) respond(ctx context.Context, data []byte, empty string) (ports.RecognitionResult, error) {
	switch {
	case bytes.Contains(data, []byte("FAIL")):
		return ports.RecognitionResult{}, fmt.Errorf("fakeai: %w", ports.ErrProviderUnavailable)
	case bytes.Contains(data, []byte("SLOW")):
		<-ctx.Done()
		return ports.RecognitionResult{}, ctx.Err()
	case bytes.Contains(data, []byte(empty)):
		return ports.RecognitionResult{Model: r.Model}, nil
	}
	return ports.RecognitionResult{
		Model: r.Model,
		Recognition: domain.FoodRecognition{Items: []domain.RecognizedFood{
			{Name: "chicken breast", DisplayName: "Pechuga de pollo", Grams: 150, Confidence: 0.85},
			{Name: "white rice", DisplayName: "Arroz blanco", Grams: 120, Confidence: 0.8},
			{Name: "dragon glaze", DisplayName: "Glaseado", Grams: 30, Confidence: 0.4},
		}},
	}, nil
}

// Phrase es lo que "escucha" el transcriptor simulado.
const Phrase = "Comí pechuga de pollo con arroz blanco y un poco de glaseado"

// Transcriber implementa ports.Transcriber.
type Transcriber struct{}

func (Transcriber) Transcribe(_ context.Context, audio ports.Audio) (ports.Transcript, error) {
	switch {
	case bytes.Contains(audio.Data, []byte("FAIL")):
		return ports.Transcript{}, fmt.Errorf("fakeai: %w", ports.ErrProviderUnavailable)
	case bytes.Contains(audio.Data, []byte("SILENCE")):
		return ports.Transcript{Model: "fake-whisper", Seconds: 10}, nil
	}
	return ports.Transcript{Text: Phrase, Seconds: 10, Model: "fake-whisper"}, nil
}

var (
	_ ports.FoodRecognizer     = Recognizer{}
	_ ports.TextFoodRecognizer = Recognizer{}
	_ ports.Transcriber        = Transcriber{}
)
