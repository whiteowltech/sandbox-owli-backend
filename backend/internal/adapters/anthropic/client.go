// Package anthropic implementa ports.FoodRecognizer con la Messages API de
// Claude. La salida estructurada se fuerza con una herramienta obligatoria
// (tool_choice) cuyo input_schema es el contrato de foodprompt.
package anthropic

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

const (
	defaultBaseURL = "https://api.anthropic.com/v1"
	apiVersion     = "2023-06-01"
	toolName       = "report_foods"
)

// Config del cliente. Model y APIKey son obligatorios.
type Config struct {
	APIKey     string
	Model      string // p. ej. "claude-haiku-5-5"
	BaseURL    string
	HTTPClient *http.Client
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
		return nil, errors.New("anthropic: falta APIKey")
	}
	if cfg.Model == "" {
		return nil, errors.New("anthropic: falta Model")
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
		return ports.RecognitionResult{}, errors.New("anthropic: imagen vacía")
	}
	return c.recognize(ctx, foodprompt.System, []contentBlock{
		{Type: "image", Source: &imageSource{Type: "base64", MediaType: img.MIMEType, Data: base64.StdEncoding.EncodeToString(img.Data)}},
		{Type: "text", Text: foodprompt.User},
	})
}

// RecognizeText identifica alimentos y gramos en lo que el usuario dictó.
// A diferencia de las fotos, aquí NO se aplica el factor de calibración de gramos:
// se midió con imágenes, y en voz el usuario suele decir la cantidad.
func (c *Client) RecognizeText(ctx context.Context, text string) (ports.RecognitionResult, error) {
	if strings.TrimSpace(text) == "" {
		return ports.RecognitionResult{}, errors.New("anthropic: texto vacío")
	}
	return c.recognize(ctx, foodprompt.TextSystem, []contentBlock{{Type: "text", Text: foodprompt.TextUser(text)}})
}

func (c *Client) recognize(ctx context.Context, system string, blocks []contentBlock) (ports.RecognitionResult, error) {
	req := messagesRequest{
		Model:     c.cfg.Model,
		MaxTokens: 1024,
		System:    system,
		Messages:  []message{{Role: "user", Content: blocks}},
		Tools: []tool{{
			Name:        toolName,
			Description: "Registra los alimentos identificados con sus gramos estimados.",
			InputSchema: foodprompt.JSONSchema,
		}},
		ToolChoice: toolChoice{Type: "tool", Name: toolName},
	}

	var resp messagesResponse
	if err := c.post(ctx, req, &resp); err != nil {
		return ports.RecognitionResult{}, err
	}
	for _, block := range resp.Content {
		if block.Type != "tool_use" || block.Name != toolName {
			continue
		}
		rec, err := foodprompt.Decode(block.Input)
		if err != nil {
			return ports.RecognitionResult{}, err
		}
		return ports.RecognitionResult{
			Recognition: rec,
			Model:       c.cfg.Model,
			Usage: ports.TokenUsage{
				InputTokens:  resp.Usage.InputTokens,
				OutputTokens: resp.Usage.OutputTokens,
			},
		}, nil
	}
	return ports.RecognitionResult{}, fmt.Errorf("anthropic: respuesta sin %s (stop_reason %q)", toolName, resp.StopReason)
}

func (c *Client) post(ctx context.Context, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("anthropic: serializar petición: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/messages", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("anthropic: crear petición: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", apiVersion)

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("anthropic: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("anthropic: leer respuesta: %w", err)
	}
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests:
		return fmt.Errorf("anthropic: %w", ports.ErrProviderRateLimited)
	case http.StatusServiceUnavailable, 529: // 529 = overloaded
		return fmt.Errorf("anthropic: %w", ports.ErrProviderUnavailable)
	default:
		return fmt.Errorf("anthropic: HTTP %d: %s", res.StatusCode, truncate(raw, 500))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("anthropic: decodificar respuesta: %w", err)
	}
	return nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// --- Tipos de la Messages API ---

type messagesRequest struct {
	Model      string     `json:"model"`
	MaxTokens  int        `json:"max_tokens"` // sin temperature: los modelos Claude 5.x la rechazan
	System     string     `json:"system"`
	Messages   []message  `json:"messages"`
	Tools      []tool     `json:"tools"`
	ToolChoice toolChoice `json:"tool_choice"`
}

type message struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type   string       `json:"type"`
	Text   string       `json:"text,omitempty"`
	Source *imageSource `json:"source,omitempty"`
}

type imageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

type toolChoice struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type messagesResponse struct {
	Content []struct {
		Type  string          `json:"type"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}
