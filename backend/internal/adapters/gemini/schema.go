package gemini

import "github.com/whiteowltech/owli/backend/internal/adapters/foodprompt"

// recognitionSchema es el responseSchema (subconjunto OpenAPI, tipos en
// mayúsculas) que fuerza la estructura definida en foodprompt.
var recognitionSchema = map[string]any{
	"type": "OBJECT",
	"properties": map[string]any{
		"items": map[string]any{
			"type": "ARRAY",
			"items": map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"name":         map[string]any{"type": "STRING"},
					"display_name": map[string]any{"type": "STRING"},
					"grams":        map[string]any{"type": "NUMBER"},
					"confidence":   map[string]any{"type": "NUMBER"},
					"per_100g":     foodprompt.Per100gSchema("OBJECT", "NUMBER"),
					"quantity":     map[string]any{"type": "NUMBER"},
					"unit_one":     map[string]any{"type": "STRING"},
					"unit_many":    map[string]any{"type": "STRING"},
					"unit_grams":   map[string]any{"type": "NUMBER"},
				},
				"required":         foodprompt.ItemFields,
				"propertyOrdering": foodprompt.ItemFields,
			},
		},
		"recipe": foodprompt.RecipeSchema("OBJECT", "BOOLEAN", "STRING", "NUMBER"),
	},
	"required":         []string{"items", "recipe"},
	"propertyOrdering": []string{"items", "recipe"},
}

// --- Tipos de la API REST generateContent ---

type generateRequest struct {
	SystemInstruction *content         `json:"systemInstruction,omitempty"`
	Contents          []content        `json:"contents"`
	GenerationConfig  generationConfig `json:"generationConfig"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inlineData,omitempty"`
}

type inlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type generationConfig struct {
	Temperature      float64         `json:"temperature"`
	ResponseMIMEType string          `json:"responseMimeType"`
	ResponseSchema   any             `json:"responseSchema"`
	ThinkingConfig   *thinkingConfig `json:"thinkingConfig,omitempty"`
}

type thinkingConfig struct {
	ThinkingLevel string `json:"thinkingLevel,omitempty"`
}

type generateResponse struct {
	Candidates []struct {
		Content content `json:"content"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
}
