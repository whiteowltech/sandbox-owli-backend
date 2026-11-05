package httpapi

import "testing"

func TestWeekEndpoint(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	expect(t, a.do("GET", "/v1/me/week", "", nil, nil), 401)
	token := readyUser(t, a)

	var w struct {
		Start string `json:"start"`
		End   string `json:"end"`
		Days  []struct {
			Date    string `json:"date"`
			Entries int    `json:"entries"`
			Mood    string `json:"mood"`
		} `json:"days"`
		Target struct {
			Kcal int `json:"kcal"`
		} `json:"target"`
		Review struct {
			LoggedDays int      `json:"loggedDays"`
			AvgKcal    *float64 `json:"avgKcal"`
			Streak     int      `json:"streak"`
		} `json:"review"`
	}
	expect(t, a.do("GET", "/v1/me/week", token, nil, &w), 200)
	if len(w.Days) != 7 || w.Days[6].Date != w.End || w.Days[0].Date != w.Start || w.Target.Kcal == 0 {
		t.Fatalf("semana vacía = %+v", w)
	}
	if w.Review.AvgKcal != nil || w.Review.LoggedDays != 0 {
		t.Errorf("sin anotaciones el promedio es null: %+v", w.Review)
	}

	expect(t, a.do("POST", "/v1/me/entries", token, map[string]any{"foodId": "pechuga-pollo", "grams": 150}, nil), 201)
	expect(t, a.do("GET", "/v1/me/week", token, nil, &w), 200)
	if w.Days[6].Entries != 1 || w.Review.LoggedDays != 1 || w.Review.AvgKcal == nil || w.Review.Streak != 1 {
		t.Errorf("con una comida hoy = %+v", w)
	}

	var e errorRes
	expect(t, a.do("GET", "/v1/me/week?end=2999-01-01", token, nil, &e), 422)
	expect(t, a.do("GET", "/v1/me/week?end=ayer", token, nil, &e), 422)
}
