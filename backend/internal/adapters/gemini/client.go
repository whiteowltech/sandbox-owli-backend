// Package gemini implementa ports.FoodRecognizer y ports.TextFoodRecognizer con la API REST de Gemini,
// usando salida JSON estructurada (responseSchema) y temperatura 0.
package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/whiteowltech/owli/backend/internal/adapters/foodprompt"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Config del cliente. Model y APIKey son obligatorios.
type Config struct {
	APIKey string
	Model  string // p. ej. "gemini-3.1-flash-lite"
	// ThinkingLevel es opcional ("minimal", "low", "medium", "high"); vacío usa el del modelo.
	ThinkingLevel string
	BaseURL       string
	HTTPClient    *http.Client
}

// Client es seguro para uso concurrente.
type Client struct {
	cfg  Config
	http *http.Client
}

var (
	_ ports.FoodRecognizer     = (*Client)(nil)
	_ ports.TextFoodRecognizer = (*Client)(nil)
)

// New valida la configuración y construye el cliente.
func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("gemini: falta APIKey")
	}
	if cfg.Model == "" {
		return nil, errors.New("gemini: falta Model")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	return &Client{cfg: cfg, http: hc}, nil
}

// RecognizeImage identifica alimentos y gramos en la imagen.
func (c *Client) RecognizeImage(ctx context.Context, img ports.Image) (ports.RecognitionResult, error) {
	if len(img.Data) == 0 {
		return ports.RecognitionResult{}, errors.New("gemini: imagen vacía")
	}
	return c.recognize(ctx, foodprompt.System, []part{
		{InlineData: &inlineData{MIMEType: img.MIMEType, Data: base64.StdEncoding.EncodeToString(img.Data)}},
		{Text: foodprompt.User},
	})
}

// RecognizeText identifica alimentos y gramos en lo que el usuario dictó (modo texto).
func (c *Client) RecognizeText(ctx context.Context, text string) (ports.RecognitionResult, error) {
	if strings.TrimSpace(text) == "" {
		return ports.RecognitionResult{}, errors.New("gemini: texto vacío")
	}
	return c.recognize(ctx, foodprompt.TextSystem, []part{{Text: foodprompt.TextUser(text)}})
}

func (c *Client) recognize(ctx context.Context, system string, parts []part) (ports.RecognitionResult, error) {
	req := generateRequest{
		SystemInstruction: &content{Parts: []part{{Text: system}}},
		Contents:          []content{{Role: "user", Parts: parts}},
		GenerationConfig: generationConfig{
			Temperature:      0,
			ResponseMIMEType: "application/json",
			ResponseSchema:   recognitionSchema,
		},
	}
	if c.cfg.ThinkingLevel != "" {
		req.GenerationConfig.ThinkingConfig = &thinkingConfig{ThinkingLevel: c.cfg.ThinkingLevel}
	}

	var resp generateResponse
	if err := c.post(ctx, req, &resp); err != nil {
		return ports.RecognitionResult{}, err
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		reason := ""
		if resp.PromptFeedback != nil {
			reason = resp.PromptFeedback.BlockReason
		}
		return ports.RecognitionResult{}, fmt.Errorf("gemini: respuesta sin contenido (bloqueo: %q)", reason)
	}

	rec, err := foodprompt.Decode([]byte(resp.Candidates[0].Content.Parts[0].Text))
	if err != nil {
		return ports.RecognitionResult{}, err
	}

	u := resp.UsageMetadata
	return ports.RecognitionResult{
		Recognition: rec,
		Model:       c.cfg.Model,
		Usage: ports.TokenUsage{
			InputTokens:    u.PromptTokenCount,
			OutputTokens:   u.CandidatesTokenCount + u.ThoughtsTokenCount,
			ThinkingTokens: u.ThoughtsTokenCount,
		},
	}, nil
}

func (c *Client) post(ctx context.Context, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("gemini: serializar petición: %w", err)
	}
	url := fmt.Sprintf("%s/models/%s:generateContent", c.cfg.BaseURL, c.cfg.Model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("gemini: crear petición: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.cfg.APIKey) // en header, nunca en la URL (no termina en logs)

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("gemini: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("gemini: leer respuesta: %w", err)
	}
	switch {
	case res.StatusCode == http.StatusTooManyRequests:
		return fmt.Errorf("gemini: %w", ports.ErrProviderRateLimited)
	case res.StatusCode == http.StatusServiceUnavailable:
		return fmt.Errorf("gemini: %w", ports.ErrProviderUnavailable)
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("gemini: HTTP %d: %s", res.StatusCode, truncate(raw, 500))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("gemini: decodificar respuesta: %w", err)
	}
	return nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
