package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"
)

// uploadPhoto envía una foto como multipart. El reconocedor simulado responde
// según el contenido ("FAIL", "EMPTY" o cualquier otro).
func (a *api) uploadPhoto(token, content, mime string, out any) *http.Response {
	a.t.Helper()
	return a.upload("/v1/me/analyses/photo", "photo", token, content, mime, out)
}

// uploadVoice envía un audio. El transcriptor simulado responde según el
// contenido ("FAIL", "SILENCE" o cualquier otro).
func (a *api) uploadVoice(token, content, mime string, out any) *http.Response {
	a.t.Helper()
	return a.upload("/v1/me/analyses/voice", "audio", token, content, mime, out)
}

func (a *api) upload(path, field, token, content, mime string, out any) *http.Response {
	a.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="`+field+`"; filename="archivo"`)
	h.Set("Content-Type", mime)
	part, _ := mw.CreatePart(h)
	_, _ = part.Write([]byte(content))
	_ = mw.Close()
	req, _ := http.NewRequest("POST", a.srv.URL+path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res
}

type creditsRes struct {
	FreeToday int `json:"freeToday"`
	Welcome   int `json:"welcome"`
	Total     int `json:"total"`
}

type analysisRes struct {
	ID    string `json:"id"`
	Items []struct {
		DisplayName string  `json:"displayName"`
		Grams       float64 `json:"grams"`
		NeedsReview bool    `json:"needsReview"`
		Food        *struct {
			ID string `json:"id"`
		} `json:"food"`
	} `json:"items"`
	NoFood     bool       `json:"noFood"`
	Transcript string     `json:"transcript"`
	Credits    creditsRes `json:"credits"`
}

func readyUser(t *testing.T, a *api) string {
	t.Helper()
	var s sessionRes
	a.do("POST", "/v1/auth/anonymous", "", nil, &s)
	expect(t, a.do("PUT", "/v1/me/profile", s.AccessToken, validProfile(), nil), 200)
	return s.AccessToken
}

func TestPhotoAnalysisFlow(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	token := readyUser(t, a)

	var c creditsRes
	expect(t, a.do("GET", "/v1/me/credits", token, nil, &c), 200)
	if c.FreeToday != 1 || c.Welcome != 15 || c.Total != 16 {
		t.Fatalf("créditos iniciales = %+v", c)
	}

	var res analysisRes
	expect(t, a.uploadPhoto(token, "plato", "image/jpeg", &res), 200)
	if len(res.Items) != 3 || res.Items[0].Food == nil || res.Items[0].Food.ID != "pechuga-pollo" {
		t.Fatalf("análisis = %+v", res)
	}
	if !res.Items[2].NeedsReview || res.Items[2].Food != nil {
		t.Errorf("la salsa (sin coincidencia, confianza baja) se marca para revisar: %+v", res.Items[2])
	}
	if res.Credits.FreeToday != 0 || res.Credits.Total != 15 {
		t.Errorf("créditos tras analizar = %+v", res.Credits)
	}

	// Confirmar: se guarda como comida de origen "photo".
	var entry struct {
		Source string `json:"source"`
	}
	expect(t, a.do("POST", "/v1/me/entries", token, map[string]any{"foodId": "pechuga-pollo", "grams": 150, "source": "photo"}, &entry), 201)
	if entry.Source != "photo" {
		t.Errorf("origen = %q", entry.Source)
	}
	var e errorRes
	expect(t, a.do("POST", "/v1/me/entries", token, map[string]any{"foodId": "pechuga-pollo", "grams": 150, "source": "telepatia"}, &e), 422)
}

func TestPhotoAnalysisFailuresDoNotCharge(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	token := readyUser(t, a)

	var e errorRes
	expect(t, a.uploadPhoto(token, "FAIL", "image/jpeg", &e), 503)
	if e.Error.Code != "ai_unavailable" {
		t.Errorf("código = %q", e.Error.Code)
	}
	var res analysisRes
	expect(t, a.uploadPhoto(token, "EMPTY", "image/jpeg", &res), 200)
	if !res.NoFood {
		t.Error("sin comida = noFood")
	}
	var c creditsRes
	a.do("GET", "/v1/me/credits", token, nil, &c)
	if c.Total != 16 {
		t.Errorf("ni la falla ni la foto sin comida cobran: %+v", c)
	}
}

func TestPhotoValidation(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	token := readyUser(t, a)
	var e errorRes
	expect(t, a.uploadPhoto(token, "plato", "application/pdf", &e), 415)
	expect(t, a.do("POST", "/v1/me/analyses/photo", token, map[string]any{}, &e), 400)
	big := string(bytes.Repeat([]byte("x"), 5<<20))
	expect(t, a.uploadPhoto(token, big, "image/jpeg", &e), 413)
	expect(t, a.uploadPhoto("", "plato", "image/jpeg", nil), 401)
}

func TestRunningOutOfCredits(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	token := readyUser(t, a)
	for range 16 {
		expect(t, a.uploadPhoto(token, "plato", "image/jpeg", nil), 200)
	}
	var e errorRes
	expect(t, a.uploadPhoto(token, "plato", "image/jpeg", &e), 402)
	if e.Error.Code != "no_credits" {
		t.Errorf("código = %q", e.Error.Code)
	}
	// Anotar a mano sigue funcionando sin créditos.
	expect(t, a.do("POST", "/v1/me/entries", token, map[string]any{"foodId": "pechuga-pollo", "grams": 100}, nil), 201)
}

func TestAIRateLimitIsSeparateFromCredits(t *testing.T) {
	a := newAPI(t, Limits{AnonymousPerIPHour: 10, SignInPerIPHour: 60, UserPerMinute: 120, AIPerUserHour: 2})
	token := readyUser(t, a)
	expect(t, a.uploadPhoto(token, "plato", "image/jpeg", nil), 200)
	expect(t, a.uploadPhoto(token, "plato", "image/jpeg", nil), 200)
	expect(t, a.uploadPhoto(token, "plato", "image/jpeg", nil), 429)
}

func TestVoiceAnalysisFlow(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	token := readyUser(t, a)

	var res analysisRes
	// Chrome graba webm con parámetros de códec: se aceptan.
	expect(t, a.uploadVoice(token, "audio", "audio/webm;codecs=opus", &res), 200)
	if res.Transcript == "" || len(res.Items) != 3 || res.Items[0].Food == nil {
		t.Fatalf("análisis por voz = %+v", res)
	}
	if res.Credits.Total != 15 {
		t.Errorf("la voz cobra 1 análisis: %+v", res.Credits)
	}

	var e errorRes
	expect(t, a.uploadVoice(token, "FAIL", "audio/mp4", &e), 503)
	expect(t, a.uploadVoice(token, "SILENCE", "audio/ogg", &res), 200)
	if !res.NoFood {
		t.Error("silencio = noFood")
	}
	var c creditsRes
	a.do("GET", "/v1/me/credits", token, nil, &c)
	if c.Total != 15 {
		t.Errorf("ni la falla ni el silencio cobran: %+v", c)
	}

	expect(t, a.uploadVoice(token, "audio", "video/mp4", &e), 415)
	expect(t, a.uploadVoice(token, string(bytes.Repeat([]byte("x"), 6<<20)), "audio/webm", &e), 413)
	expect(t, a.uploadVoice("", "audio", "audio/webm", nil), 401)
}

func TestTranscriptAnalysis(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	token := readyUser(t, a)

	var c struct {
		Total int `json:"total"`
		Voice struct {
			Transcript bool `json:"transcript"`
			Audio      bool `json:"audio"`
		} `json:"voice"`
	}
	expect(t, a.do("GET", "/v1/me/credits", token, nil, &c), 200)
	if c.Total != 16 || !c.Voice.Transcript || !c.Voice.Audio {
		t.Fatalf("créditos y voz = %+v", c)
	}

	var res analysisRes
	expect(t, a.do("POST", "/v1/me/analyses/text", token, map[string]any{"text": "dos empanadas de carne"}, &res), 200)
	if res.Transcript != "dos empanadas de carne" || len(res.Items) != 3 || res.Credits.Total != 15 {
		t.Fatalf("análisis de texto = %+v", res)
	}
	var e errorRes
	expect(t, a.do("POST", "/v1/me/analyses/text", token, map[string]any{"text": " "}, &e), 422)
	if e.Error.Code != "invalid_transcript" {
		t.Errorf("código = %q", e.Error.Code)
	}
	expect(t, a.do("POST", "/v1/me/analyses/text", "", map[string]any{"text": "pollo"}, nil), 401)
}
