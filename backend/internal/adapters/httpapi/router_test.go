package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/whiteowltech/owli/backend/internal/adapters/fakeai"
	"github.com/whiteowltech/owli/backend/internal/adapters/jwtauth"
	"github.com/whiteowltech/owli/backend/internal/adapters/memory"
	"github.com/whiteowltech/owli/backend/internal/adapters/redisx"
	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
	"github.com/whiteowltech/owli/backend/internal/usecase"
)

// fakeApple acepta tokens "ok:<subject>".
type fakeApple struct{}

func (fakeApple) Verify(_ context.Context, token, _ string) (ports.ExternalIdentity, error) {
	if sub, ok := strings.CutPrefix(token, "ok:"); ok {
		return ports.ExternalIdentity{Provider: ports.ProviderApple, Subject: sub}, nil
	}
	return ports.ExternalIdentity{}, ports.ErrInvalidCredentials
}

type api struct {
	t     *testing.T
	srv   *httptest.Server
	store *memory.Store
}

func newAPI(t *testing.T, limits Limits) *api {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := memory.NewStore()
	jwt, err := jwtauth.New(strings.Repeat("s", 40), "", jwtauth.DefaultTTL, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	mr := miniredis.RunT(t)
	auth := usecase.NewAuthService(store, store, jwt,
		map[ports.IdentityProvider]ports.IdentityVerifier{ports.ProviderApple: fakeApple{}}, usecase.DefaultRefreshTTL, time.Now)
	store.AddFood(domain.Food{ID: "arroz-blanco", NameES: "Arroz blanco cocido", NameEN: "Cooked white rice",
		Per100g: domain.Macros{Calories: 130, ProteinG: 2.7, CarbsG: 28.2, FatG: 0.3}})
	store.AddFood(domain.Food{ID: "pechuga-pollo", NameES: "Pechuga de pollo asada", NameEN: "Roasted chicken breast",
		Per100g: domain.Macros{Calories: 165, ProteinG: 31, FatG: 3.6}})
	diary := usecase.NewDiaryService(store.FoodsRepo(), store.EntriesRepo(), store, time.Now)
	// Platos con atributos reales: el perfil de prueba es paleo y sin gluten.
	chicken := domain.Food{ID: "pechuga-pollo", NameES: "Pechuga de pollo asada", Per100g: domain.Macros{Calories: 165, ProteinG: 31, FatG: 3.6},
		Traits: domain.FoodTraits{AnimalOrigin: true, ContainsMeat: true, AllergensVerified: true}}
	sweetPotato := domain.Food{ID: "batata", NameES: "Batata", Per100g: domain.Macros{Calories: 90, ProteinG: 2, CarbsG: 20.7, FiberG: 3.3, FatG: 0.2},
		Traits: domain.FoodTraits{PlantOrigin: true, AllergensVerified: true}}
	rice := domain.Food{ID: "arroz-blanco", NameES: "Arroz blanco cocido", Per100g: domain.Macros{Calories: 130, ProteinG: 2.7, CarbsG: 28.2, FatG: 0.3},
		Traits: domain.FoodTraits{PlantOrigin: true, ContainsGrain: true, AllergensVerified: true}}
	dishes := []domain.DishTemplate{
		{ID: "pollo-batata", NameES: "Pollo con batata", NameEN: "Chicken with sweet potato",
			Components: []domain.DishComponent{{Food: chicken, MinG: 60, MaxG: 300}, {Food: sweetPotato, MinG: 60, MaxG: 350}}},
		{ID: "pollo-arroz", NameES: "Pollo con arroz", NameEN: "Chicken with rice", // paleo: no
			Components: []domain.DishComponent{{Food: chicken, MinG: 60, MaxG: 300}, {Food: rice, MinG: 30, MaxG: 350}}},
	}
	h := NewRouter(Deps{
		Logger: log, Auth: auth, Profiles: usecase.NewProfileService(store), Verifier: jwt,
		Diary:       diary,
		Suggestions: usecase.NewSuggestionService(diary, store, dishes, time.Now),
		Analysis: usecase.NewAnalysisService(memory.NewCredits(),
			domain.CreditPolicy{FreeDailyQuota: 1, WelcomeCredits: 15, ProFairUseDaily: 25},
			store, store.FoodsRepo(), redisx.NewSpendGuard(redis.NewClient(&redis.Options{Addr: mr.Addr()}), 20),
			fakeai.Recognizer{Model: "fake"}, nil,
			usecase.AIConfig{PrimaryTimeout: time.Second, FallbackTimeout: time.Second, FallbackGramsFactor: 0.71, CostMicroUSD: 3000,
				VoiceCostMicroUSD: 1000, TranscribeTimeout: time.Second},
			log, time.Now).WithVoice(fakeai.Transcriber{}, fakeai.Recognizer{Model: "fake"}, nil),
		Limiter: redisx.NewRateLimiter(redis.NewClient(&redis.Options{Addr: mr.Addr()})), Limits: limits,
		AllowedOrigins: []string{"http://localhost:8100"},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &api{t: t, srv: srv, store: store}
}

// do envía un pedido y decodifica la respuesta JSON (si hay) en out.
func (a *api) do(method, path, token string, body any, out any) *http.Response {
	a.t.Helper()
	var r io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		r = strings.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		r = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, a.srv.URL+path, r)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res
}

type sessionRes struct {
	UserID       string `json:"userId"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	Created      *bool  `json:"created"`
}

type errorRes struct {
	Error struct{ Code string } `json:"error"`
}

func expect(t *testing.T, res *http.Response, status int) {
	t.Helper()
	if res.StatusCode != status {
		t.Fatalf("%s %s = %d, se esperaba %d", res.Request.Method, res.Request.URL.Path, res.StatusCode, status)
	}
}

func validProfile() map[string]any {
	return map[string]any{
		"name": "Leandro", "sex": "male", "ageYears": 30, "heightCm": 175, "weightKg": 80, "activity": "moderate",
		"goal": "cut", "pace": "steady", "targetWeightKg": 74, "diet": "paleo", "restrictions": []string{"gluten_free"},
		"mealsPerDay": 4, "units": "metric", "timezone": "America/Argentina/Buenos_Aires",
	}
}

func TestHealthz(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	expect(t, a.do("GET", "/healthz", "", nil, nil), 200)
}

func TestAnonymousAccountSavesProfile(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	var s sessionRes
	expect(t, a.do("POST", "/v1/auth/anonymous", "", nil, &s), 201)
	if s.AccessToken == "" || s.RefreshToken == "" {
		t.Fatalf("sesión incompleta: %+v", s)
	}

	var e errorRes
	expect(t, a.do("GET", "/v1/me/profile", s.AccessToken, nil, &e), 404)

	var saved struct {
		Profile map[string]any `json:"profile"`
		Plan    struct {
			DailyTarget int `json:"dailyTarget"`
			TDEE        int `json:"tdee"`
		} `json:"plan"`
	}
	expect(t, a.do("PUT", "/v1/me/profile", s.AccessToken, validProfile(), &saved), 200)
	if saved.Plan.TDEE != 2711 || saved.Plan.DailyTarget != 2161 { // caso de testdata/plan_cases.json
		t.Errorf("plan = %+v", saved.Plan)
	}
	var got struct {
		Profile map[string]any `json:"profile"`
	}
	expect(t, a.do("GET", "/v1/me/profile", s.AccessToken, nil, &got), 200)
	if got.Profile["diet"] != "paleo" || got.Profile["timezone"] != "America/Argentina/Buenos_Aires" {
		t.Errorf("perfil leído = %v", got.Profile)
	}
}

func TestProfileValidationAndBadRequests(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	var s sessionRes
	a.do("POST", "/v1/auth/anonymous", "", nil, &s)

	minor := validProfile()
	minor["ageYears"] = 16
	var e errorRes
	expect(t, a.do("PUT", "/v1/me/profile", s.AccessToken, minor, &e), 422)
	if e.Error.Code != "underage" {
		t.Errorf("código = %q", e.Error.Code)
	}

	badDiet := validProfile()
	badDiet["diet"] = "frugivoro"
	expect(t, a.do("PUT", "/v1/me/profile", s.AccessToken, badDiet, &e), 422)

	withUserID := validProfile()
	withUserID["userId"] = "otro-usuario" // nunca se acepta un user_id en el body
	expect(t, a.do("PUT", "/v1/me/profile", s.AccessToken, withUserID, &e), 400)

	expect(t, a.do("PUT", "/v1/me/profile", s.AccessToken, "{no es json", &e), 400)
	expect(t, a.do("PUT", "/v1/me/profile", s.AccessToken, `{"name":"a"} {"name":"b"}`, &e), 400)
	expect(t, a.do("PUT", "/v1/me/profile", s.AccessToken, strings.Repeat(" ", 20<<10)+"{}", &e), 400)
}

func TestAuthenticationIsRequired(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	var e errorRes
	expect(t, a.do("GET", "/v1/me/profile", "", nil, &e), 401)
	if e.Error.Code != "unauthenticated" {
		t.Errorf("sin token = %q", e.Error.Code)
	}
	expect(t, a.do("GET", "/v1/me/profile", "token.falso.x", nil, &e), 401)
	expect(t, a.do("DELETE", "/v1/me", "", nil, nil), 401)
}

func TestUsersOnlySeeTheirOwnProfile(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	var alice, bob sessionRes
	a.do("POST", "/v1/auth/anonymous", "", nil, &alice)
	a.do("POST", "/v1/auth/anonymous", "", nil, &bob)
	expect(t, a.do("PUT", "/v1/me/profile", alice.AccessToken, validProfile(), nil), 200)
	expect(t, a.do("GET", "/v1/me/profile", bob.AccessToken, nil, nil), 404)
}

func TestRefreshRotationAndReuse(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	var s1, s2 sessionRes
	a.do("POST", "/v1/auth/anonymous", "", nil, &s1)
	expect(t, a.do("POST", "/v1/auth/refresh", "", map[string]string{"refreshToken": s1.RefreshToken}, &s2), 200)
	if s2.UserID != s1.UserID || s2.RefreshToken == s1.RefreshToken {
		t.Fatal("la renovación mantiene el usuario y rota el token")
	}
	var e errorRes
	expect(t, a.do("POST", "/v1/auth/refresh", "", map[string]string{"refreshToken": s1.RefreshToken}, &e), 401)
	if e.Error.Code != "invalid_session" {
		t.Errorf("reuso = %q", e.Error.Code)
	}
	expect(t, a.do("POST", "/v1/auth/logout", "", map[string]string{"refreshToken": "x"}, nil), 204)
}

func TestSignInAndLink(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	var first sessionRes
	expect(t, a.do("POST", "/v1/auth/signin/apple", "", map[string]string{"idToken": "ok:apple-1", "nonce": "n"}, &first), 200)
	if first.Created == nil || !*first.Created {
		t.Error("la primera vez crea la cuenta")
	}

	var anon sessionRes
	a.do("POST", "/v1/auth/anonymous", "", nil, &anon)
	expect(t, a.do("POST", "/v1/auth/link/apple", anon.AccessToken, map[string]string{"idToken": "ok:apple-2"}, nil), 204)
	var e errorRes
	expect(t, a.do("POST", "/v1/auth/link/apple", anon.AccessToken, map[string]string{"idToken": "ok:apple-1"}, &e), 409)
	if e.Error.Code != "identity_in_use" {
		t.Errorf("conflicto = %q", e.Error.Code)
	}
	expect(t, a.do("POST", "/v1/auth/signin/apple", "", map[string]string{"idToken": "falso"}, &e), 401)
	expect(t, a.do("POST", "/v1/auth/signin/google", "", map[string]string{"idToken": "ok:g"}, &e), 400)
	if e.Error.Code != "provider_not_available" {
		t.Errorf("Google sin configurar = %q", e.Error.Code)
	}
}

func TestDeleteAccount(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	var s sessionRes
	a.do("POST", "/v1/auth/anonymous", "", nil, &s)
	expect(t, a.do("PUT", "/v1/me/profile", s.AccessToken, validProfile(), nil), 200)
	expect(t, a.do("DELETE", "/v1/me", s.AccessToken, nil, nil), 204)
	// El access token sigue vigente unos minutos, pero ya no hay datos ni se pueden crear.
	expect(t, a.do("GET", "/v1/me/profile", s.AccessToken, nil, nil), 404)
	expect(t, a.do("PUT", "/v1/me/profile", s.AccessToken, validProfile(), nil), 404)
	expect(t, a.do("POST", "/v1/auth/refresh", "", map[string]string{"refreshToken": s.RefreshToken}, nil), 401)
}

func TestAnonymousAccountsAreRateLimitedPerIP(t *testing.T) {
	a := newAPI(t, Limits{AnonymousPerIPHour: 3, SignInPerIPHour: 60, UserPerMinute: 120})
	for range 3 {
		expect(t, a.do("POST", "/v1/auth/anonymous", "", nil, nil), 201)
	}
	res := a.do("POST", "/v1/auth/anonymous", "", nil, nil)
	expect(t, res, 429)
	if res.Header.Get("Retry-After") == "" {
		t.Error("falta Retry-After")
	}
}

func TestCORS(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	pre := func(origin string) *http.Response {
		req, _ := http.NewRequest("OPTIONS", a.srv.URL+"/v1/me/profile", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "PUT")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}
	if got := pre("http://localhost:8100").Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:8100" {
		t.Errorf("origen permitido = %q", got)
	}
	if got := pre("https://evil.example").Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("origen ajeno no debe habilitarse: %q", got)
	}
}
