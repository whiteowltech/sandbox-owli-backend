package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

// maxPhotoBytes: la app comprime la foto antes de subirla (≤ 1024 px, JPEG);
// 4 MB deja margen para teléfonos que no la reducen.
const maxPhotoBytes = 4 << 20

var allowedPhotoTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}

// maxVoiceBytes: la app corta la grabación a los 60 s (Opus ~ 32 kbit/s ≈ 240 KB);
// 5 MB deja margen para formatos menos comprimidos (m4a de iOS).
const maxVoiceBytes = 5 << 20

// allowedVoiceTypes: lo que graban los navegadores (webm/ogg en Android y escritorio,
// mp4 en iOS) más los formatos que acepta Whisper. Se compara sin los parámetros (";codecs=…").
var allowedVoiceTypes = map[string]bool{
	"audio/webm": true, "audio/ogg": true, "audio/mp4": true, "audio/m4a": true, "audio/x-m4a": true,
	"audio/mpeg": true, "audio/wav": true, "audio/x-wav": true,
}

type analysisHandlers struct {
	analysis ports.AnalysisUseCase
	h        *handlers
}

type walletDTO struct {
	FreeToday int  `json:"freeToday"` // análisis gratis que quedan hoy
	Welcome   int  `json:"welcome"`
	Packs     int  `json:"packs"`
	Total     int  `json:"total"`
	IsPro     bool `json:"isPro"`
}

func toWalletDTO(w domain.CreditWallet, p domain.CreditPolicy) walletDTO {
	quota := p.FreeDailyQuota
	if w.IsPro {
		quota = p.ProFairUseDaily
	}
	free := max(0, quota-w.UsedToday)
	return walletDTO{FreeToday: free, Welcome: w.WelcomeBalance, Packs: w.PackBalance, Total: free + w.WelcomeBalance + w.PackBalance, IsPro: w.IsPro}
}

type analysisItemDTO struct {
	Name        string   `json:"name"`        // genérico, en inglés
	DisplayName string   `json:"displayName"` // para mostrar
	Grams       float64  `json:"grams"`
	Confidence  float64  `json:"confidence"`
	NeedsReview bool     `json:"needsReview"`
	Food        *foodDTO `json:"food"` // null: el usuario lo elige a mano
	// Unidades ("2 huevos"): quantity 0 = solo gramos. grams = quantity × unitGrams.
	Quantity  float64 `json:"quantity,omitempty"`
	UnitOne   string  `json:"unitOne,omitempty"`
	UnitMany  string  `json:"unitMany,omitempty"`
	UnitGrams float64 `json:"unitGrams,omitempty"`
}

type recipeInfoDTO struct {
	Name     string  `json:"name"`
	Servings float64 `json:"servings"` // 0 = no se sabe
}

type analysisDTO struct {
	ID         string            `json:"id"`
	Items      []analysisItemDTO `json:"items"`
	Transcript string            `json:"transcript,omitempty"` // solo en voz
	Recipe     *recipeInfoDTO    `json:"recipe,omitempty"`     // es una receta: la app pregunta cuánto se comió
	NoFood     bool              `json:"noFood"`
	Fallback   bool              `json:"fallback"`
	Credits    walletDTO         `json:"credits"`
}

// creditsDTO es el saldo más qué caminos de voz acepta el servidor: la app
// transcribe en el dispositivo si puede y, si no, sube el audio (si hay Whisper).
type creditsDTO struct {
	walletDTO
	Voice struct {
		Transcript bool `json:"transcript"`
		Audio      bool `json:"audio"`
	} `json:"voice"`
}

func (a *analysisHandlers) credits(w http.ResponseWriter, r *http.Request) {
	wallet, policy, err := a.analysis.Credits(r.Context(), userID(r.Context()))
	if err != nil {
		fail(w, a.h.log, err)
		return
	}
	out := creditsDTO{walletDTO: toWalletDTO(wallet, policy)}
	v := a.analysis.VoiceAvailability()
	out.Voice.Transcript, out.Voice.Audio = v.Transcript, v.Audio
	writeJSON(w, http.StatusOK, out)
}

// analyzePhoto recibe la foto como multipart (campo "photo"). La foto no se
// guarda en ningún lado: se lee en memoria y se descarta al responder.
func (a *analysisHandlers) analyzePhoto(w http.ResponseWriter, r *http.Request) {
	data, mime, ok := readUpload(w, r, "photo", maxPhotoBytes, func(m string) bool { return allowedPhotoTypes[m] }, uploadErrors{
		tooLarge:    apiError{http.StatusRequestEntityTooLarge, "photo_too_large", "La foto es demasiado grande (máximo 4 MB)."},
		missing:     apiError{http.StatusBadRequest, "bad_request", "Falta la foto (campo \"photo\")."},
		unsupported: apiError{http.StatusUnsupportedMediaType, "unsupported_photo", "La foto debe ser JPEG, PNG o WebP."},
	})
	if !ok {
		return
	}
	res, err := a.analysis.AnalyzePhoto(r.Context(), userID(r.Context()), ports.Image{Data: data, MIMEType: mime})
	a.respond(w, r, res, err)
}

// analyzeVoice recibe el audio como multipart (campo "audio"). Tampoco se guarda.
func (a *analysisHandlers) analyzeVoice(w http.ResponseWriter, r *http.Request) {
	data, mime, ok := readUpload(w, r, "audio", maxVoiceBytes, func(m string) bool {
		base, _, _ := strings.Cut(m, ";")
		return allowedVoiceTypes[strings.TrimSpace(base)]
	}, uploadErrors{
		tooLarge:    apiError{http.StatusRequestEntityTooLarge, "audio_too_large", "El audio es demasiado largo (máximo 5 MB)."},
		missing:     apiError{http.StatusBadRequest, "bad_request", "Falta el audio (campo \"audio\")."},
		unsupported: apiError{http.StatusUnsupportedMediaType, "unsupported_audio", "Formato de audio no soportado."},
	})
	if !ok {
		return
	}
	res, err := a.analysis.AnalyzeVoice(r.Context(), userID(r.Context()), ports.Audio{Data: data, MIMEType: mime})
	a.respond(w, r, res, err)
}

// analyzeText recibe lo que el dispositivo ya transcribió: {"text": "dos empanadas de carne"}.
func (a *analysisHandlers) analyzeText(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &req) {
		return
	}
	res, err := a.analysis.AnalyzeTranscript(r.Context(), userID(r.Context()), req.Text)
	a.respond(w, r, res, err)
}

type uploadErrors struct{ tooLarge, missing, unsupported apiError }

// readUpload lee un archivo multipart en memoria, con tope de tamaño y tipo permitido.
// Si algo falla, ya respondió el error y devuelve ok = false.
func readUpload(w http.ResponseWriter, r *http.Request, field string, maxBytes int64, allowed func(string) bool, e uploadErrors) ([]byte, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+64<<10)
	file, header, err := r.FormFile(field)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, nil, e.tooLarge)
			return nil, "", false
		}
		writeError(w, nil, e.missing)
		return nil, "", false
	}
	defer file.Close()
	mime := header.Header.Get("Content-Type")
	if !allowed(mime) {
		writeError(w, nil, e.unsupported)
		return nil, "", false
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || len(data) == 0 || int64(len(data)) > maxBytes {
		writeError(w, nil, e.tooLarge)
		return nil, "", false
	}
	return data, mime, true
}

func (a *analysisHandlers) respond(w http.ResponseWriter, r *http.Request, res ports.Analysis, err error) {
	if err != nil {
		fail(w, a.h.log, err)
		return
	}
	_, policy, _ := a.analysis.Credits(r.Context(), userID(r.Context()))
	out := analysisDTO{ID: res.ID, NoFood: res.NoFood, Fallback: res.Fallback, Transcript: res.Transcript,
		Items: []analysisItemDTO{}, Credits: toWalletDTO(res.Wallet, policy)}
	for _, it := range res.Items {
		rec := it.Recognized
		item := analysisItemDTO{Name: rec.Name, DisplayName: rec.DisplayName, Grams: rec.Grams,
			Confidence: rec.Confidence, NeedsReview: it.NeedsReview}
		if rec.HasUnits() {
			item.Quantity, item.UnitOne, item.UnitMany, item.UnitGrams = rec.Quantity, rec.UnitOne, rec.UnitMany, rec.UnitGrams
		}
		if it.Food != nil {
			f := toFoodDTO(*it.Food)
			item.Food = &f
		}
		out.Items = append(out.Items, item)
	}
	if rc := res.Recipe; rc != nil {
		out.Recipe = &recipeInfoDTO{Name: rc.Name, Servings: rc.Servings}
	}
	writeJSON(w, http.StatusOK, out)
}
