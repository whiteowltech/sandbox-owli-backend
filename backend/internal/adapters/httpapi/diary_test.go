package httpapi

import (
	"testing"
	"time"
)

type dayRes struct {
	Date    string `json:"date"`
	Entries []struct {
		ID     string `json:"id"`
		NameES string `json:"nameEs"`
		Grams  float64
		Macros struct {
			Kcal float64 `json:"kcal"`
		} `json:"macros"`
	} `json:"entries"`
	Totals struct {
		Kcal     float64 `json:"kcal"`
		ProteinG float64 `json:"proteinG"`
	} `json:"totals"`
	Micros map[string]struct {
		Value *float64 `json:"value"`
		Known int      `json:"known"`
	} `json:"micros"`
	Plan struct {
		DailyTarget int `json:"dailyTarget"`
	} `json:"plan"`
}

func TestManualLoggingFlow(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	var s sessionRes
	a.do("POST", "/v1/auth/anonymous", "", nil, &s)

	var found struct {
		Foods []struct {
			ID      string `json:"id"`
			Per100g struct {
				Kcal float64 `json:"kcal"`
			} `json:"per100g"`
		} `json:"foods"`
	}
	expect(t, a.do("GET", "/v1/foods?q=pollo", s.AccessToken, nil, &found), 200)
	if len(found.Foods) != 1 || found.Foods[0].ID != "pechuga-pollo" || found.Foods[0].Per100g.Kcal != 165 {
		t.Fatalf("búsqueda = %+v", found)
	}
	expect(t, a.do("GET", "/v1/foods?q=pollo", "", nil, nil), 401) // buscar también requiere sesión

	// Sin perfil, el día no se puede armar (falta la zona horaria y el objetivo).
	var e errorRes
	expect(t, a.do("GET", "/v1/me/day", s.AccessToken, nil, &e), 409)
	if e.Error.Code != "profile_required" {
		t.Errorf("código = %q", e.Error.Code)
	}
	expect(t, a.do("PUT", "/v1/me/profile", s.AccessToken, validProfile(), nil), 200)

	var entry struct {
		ID     string `json:"id"`
		Macros struct {
			Kcal float64 `json:"kcal"`
		} `json:"macros"`
	}
	expect(t, a.do("POST", "/v1/me/entries", s.AccessToken, map[string]any{"foodId": "pechuga-pollo", "grams": 200}, &entry), 201)
	if entry.ID == "" || entry.Macros.Kcal != 330 {
		t.Fatalf("comida = %+v", entry)
	}

	var day dayRes
	expect(t, a.do("GET", "/v1/me/day", s.AccessToken, nil, &day), 200)
	if len(day.Entries) != 1 || day.Totals.Kcal != 330 || day.Plan.DailyTarget != 2161 {
		t.Errorf("día = %+v", day)
	}
	if m, ok := day.Micros["ironMg"]; !ok || m.Value != nil || m.Known != 0 {
		t.Errorf("hierro sin dato debe ser null con 0 conocidos: %+v", m)
	}

	expect(t, a.do("DELETE", "/v1/me/entries/"+entry.ID, s.AccessToken, nil, nil), 204)
	expect(t, a.do("DELETE", "/v1/me/entries/"+entry.ID, s.AccessToken, nil, nil), 404)
	expect(t, a.do("GET", "/v1/me/day", s.AccessToken, nil, &day), 200)
	if len(day.Entries) != 0 || day.Totals.Kcal != 0 {
		t.Errorf("tras borrar = %+v", day)
	}
}

func TestLoggingValidation(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	var s sessionRes
	a.do("POST", "/v1/auth/anonymous", "", nil, &s)
	var e errorRes
	expect(t, a.do("POST", "/v1/me/entries", s.AccessToken, map[string]any{"foodId": "pechuga-pollo", "grams": 0}, &e), 422)
	if e.Error.Code != "invalid_entry" {
		t.Errorf("0 g = %q", e.Error.Code)
	}
	expect(t, a.do("POST", "/v1/me/entries", s.AccessToken, map[string]any{"foodId": "no-existe", "grams": 100}, &e), 404)
	future := time.Now().Add(2 * time.Hour).Format(time.RFC3339)
	expect(t, a.do("POST", "/v1/me/entries", s.AccessToken, map[string]any{"foodId": "pechuga-pollo", "grams": 100, "eatenAt": future}, &e), 422)
	expect(t, a.do("POST", "/v1/me/entries", s.AccessToken, map[string]any{"foodId": "pechuga-pollo", "grams": 100, "userId": "otro"}, &e), 400)
}

func TestEntriesAreIsolatedBetweenUsers(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	var alice, bob sessionRes
	a.do("POST", "/v1/auth/anonymous", "", nil, &alice)
	a.do("POST", "/v1/auth/anonymous", "", nil, &bob)
	var entry struct {
		ID string `json:"id"`
	}
	a.do("POST", "/v1/me/entries", alice.AccessToken, map[string]any{"foodId": "pechuga-pollo", "grams": 100}, &entry)
	expect(t, a.do("DELETE", "/v1/me/entries/"+entry.ID, bob.AccessToken, nil, nil), 404)
	expect(t, a.do("PUT", "/v1/me/profile", bob.AccessToken, validProfile(), nil), 200)
	var day dayRes
	a.do("GET", "/v1/me/day", bob.AccessToken, nil, &day)
	if len(day.Entries) != 0 {
		t.Error("Bob no ve las comidas de Alice")
	}
}
