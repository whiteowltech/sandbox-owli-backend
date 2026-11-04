package httpapi

import "testing"

func TestSuggestions(t *testing.T) {
	a := newAPI(t, DefaultLimits)
	var e errorRes
	expect(t, a.do("GET", "/v1/me/suggestions", "", nil, &e), 401)

	var s sessionRes
	a.do("POST", "/v1/auth/anonymous", "", nil, &s)
	expect(t, a.do("GET", "/v1/me/suggestions", s.AccessToken, nil, &e), 409) // primero el perfil
	expect(t, a.do("PUT", "/v1/me/profile", s.AccessToken, validProfile(), nil), 200)

	var res struct {
		MealsLeft int `json:"mealsLeft"`
		Need      struct {
			Kcal     float64 `json:"kcal"`
			ProteinG float64 `json:"proteinG"`
		} `json:"need"`
		Closed bool `json:"closed"`
		Dishes []struct {
			ID       string `json:"id"`
			NameES   string `json:"nameEs"`
			Portions []struct {
				Grams float64 `json:"grams"`
				Food  struct {
					ID string `json:"id"`
				} `json:"food"`
			} `json:"portions"`
		} `json:"dishes"`
	}
	expect(t, a.do("GET", "/v1/me/suggestions", s.AccessToken, nil, &res), 200)
	if res.MealsLeft < 1 || res.Need.Kcal <= 0 || res.Closed {
		t.Fatalf("respuesta = %+v", res)
	}
	if len(res.Dishes) != 1 || len(res.Dishes[0].Portions) != 2 || res.Dishes[0].ID != "pollo-batata" || res.Dishes[0].Portions[0].Food.ID != "pechuga-pollo" || res.Dishes[0].Portions[0].Grams <= 0 {
		t.Errorf("solo el plato paleo y sin gluten: %+v", res.Dishes)
	}
}
