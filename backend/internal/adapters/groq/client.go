// Package groq implementa ports.Transcriber con Whisper en Groq (API compatible
// con OpenAI: POST /audio/transcriptions). Solo transcribe: la extracción de
// alimentos la hace después un modelo de texto (pipeline desacoplado, CLAUDE.md).
package groq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/whiteowltech/owli/backend/internal/ports"
)

const defaultBaseURL = "https://api.groq.com/openai/v1"

// Config del cliente. Model y APIKey son obligatorios.
type Config struct {
	APIKey string
	Model  string // p. ej. "whisper-large-v3-turbo"
	// Prompt orienta el vocabulario de Whisper (nombres de comidas regionales).
	Prompt     string
	BaseURL    string
	HTTPClient *http.Client
}

// Client es seguro para uso concurrente.
type Client struct {
	cfg  Config
	http *http.Client
}

var _ ports.Transcriber = (*Client)(nil)

// DefaultPrompt sesga a Whisper hacia el vocabulario de comidas, sin forzar el idioma.
const DefaultPrompt = "Comí milanesa con puré, dos empanadas de carne, un choripán con chimichurri, ñoquis, mate, medialunas."

// New valida la configuración y construye el cliente.
func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("groq: falta APIKey")
	}
	if cfg.Model == "" {
		return nil, errors.New("groq: falta Model")
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

// Transcribe pasa el audio a texto.
func (c *Client) Transcribe(ctx context.Context, audio ports.Audio) (ports.Transcript, error) {
	if len(audio.Data) == 0 {
		return ports.Transcript{}, errors.New("groq: audio vacío")
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="audio%s"`, extension(audio.MIMEType)))
	h.Set("Content-Type", audio.MIMEType)
	fw, err := w.CreatePart(h)
	if err != nil {
		return ports.Transcript{}, fmt.Errorf("groq: armar petición: %w", err)
	}
	if _, err := fw.Write(audio.Data); err != nil {
		return ports.Transcript{}, fmt.Errorf("groq: armar petición: %w", err)
	}
	fields := map[string]string{"model": c.cfg.Model, "response_format": "verbose_json", "temperature": "0"}
	if c.cfg.Prompt != "" {
		fields["prompt"] = c.cfg.Prompt
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return ports.Transcript{}, fmt.Errorf("groq: armar petición: %w", err)
		}
	}
	if err := w.Close(); err != nil {
		return ports.Transcript{}, fmt.Errorf("groq: armar petición: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/audio/transcriptions", &body)
	if err != nil {
		return ports.Transcript{}, fmt.Errorf("groq: crear petición: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	res, err := c.http.Do(req)
	if err != nil {
		return ports.Transcript{}, fmt.Errorf("groq: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return ports.Transcript{}, fmt.Errorf("groq: leer respuesta: %w", err)
	}
	switch {
	case res.StatusCode == http.StatusTooManyRequests:
		return ports.Transcript{}, fmt.Errorf("groq: %w", ports.ErrProviderRateLimited)
	case res.StatusCode >= 500:
		return ports.Transcript{}, fmt.Errorf("groq: HTTP %d: %w", res.StatusCode, ports.ErrProviderUnavailable)
	case res.StatusCode != http.StatusOK:
		return ports.Transcript{}, fmt.Errorf("groq: HTTP %d: %s", res.StatusCode, truncate(raw, 500))
	}
	var out struct {
		Text     string  `json:"text"`
		Duration float64 `json:"duration"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return ports.Transcript{}, fmt.Errorf("groq: decodificar respuesta: %w", err)
	}
	return ports.Transcript{Text: strings.TrimSpace(out.Text), Seconds: out.Duration, Model: c.cfg.Model}, nil
}

// extension da a Whisper un nombre de archivo con la extensión correcta: la usa
// para detectar el formato.
func extension(mime string) string {
	base, _, _ := strings.Cut(mime, ";")
	switch strings.TrimSpace(base) {
	case "audio/webm":
		return ".webm"
	case "audio/ogg":
		return ".ogg"
	case "audio/mp4", "audio/m4a", "audio/x-m4a":
		return ".m4a"
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	}
	return ""
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
