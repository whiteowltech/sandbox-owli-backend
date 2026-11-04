package catalog_test

import (
	"strings"
	"testing"

	"github.com/whiteowltech/owli/backend/data"
	"github.com/whiteowltech/owli/backend/internal/catalog"
	"github.com/whiteowltech/owli/backend/internal/domain"
)

// El catálogo real de platos debe resolverse contra el catálogo real de alimentos
// (si alguien renombra un alimento, esta prueba lo avisa antes de que la API no arranque).
func TestRealDishesResolve(t *testing.T) {
	foods, err := catalog.Parse(data.FoodsSeed)
	if err != nil {
		t.Fatal(err)
	}
	dishes, err := catalog.ParseDishes(data.Dishes, foods)
	if err != nil {
		t.Fatal(err)
	}
	if len(dishes) < 40 {
		t.Errorf("platos = %d", len(dishes))
	}

	// Cada dieta tiene opciones para un almuerzo típico.
	need := domain.Macros{Calories: 650, ProteinG: 45, CarbsG: 60, FatG: 22}
	for _, diet := range []domain.DietType{domain.DietOmnivore, domain.DietVegetarian, domain.DietVegan, domain.DietPescetarian, domain.DietPaleo} {
		got := domain.SuggestDishes(dishes, domain.ClosingRequest{Need: need, Diet: diet})
		if len(got) < 3 {
			t.Errorf("%s: solo %d sugerencias", diet, len(got))
		}
	}
	lowCarb := domain.Macros{Calories: 650, ProteinG: 45, CarbsG: 5, FatG: 48}
	for _, diet := range []domain.DietType{domain.DietKeto, domain.DietCarnivore} {
		got := domain.SuggestDishes(dishes, domain.ClosingRequest{Need: lowCarb, Diet: diet})
		if len(got) < 3 {
			t.Errorf("%s: solo %d sugerencias", diet, len(got))
		}
		for _, s := range got {
			if s.Macros.NetCarbsG() > 15 {
				t.Errorf("%s: %s tiene %.0f g de carbos netos", diet, s.ID, s.Macros.NetCarbsG())
			}
		}
	}
}

func TestParseDishesRejectsBrokenRows(t *testing.T) {
	foods, _ := catalog.Parse(data.FoodsSeed)
	header := "id,name_es,name_en,breakfast,components\n"
	tests := map[string]string{
		"alimento inexistente": "x,X,X,0,pechuga-pollo:60:300|unicornio:10:20\n",
		"porciones al revés":   "x,X,X,0,pechuga-pollo:300:60|arroz-blanco:30:350\n",
		"un solo ingrediente":  "x,X,X,0,pechuga-pollo:60:300\n",
		"formato":              "x,X,X,0,pechuga-pollo-60-300|arroz-blanco:30:350\n",
		"breakfast":            "x,X,X,si,pechuga-pollo:60:300|arroz-blanco:30:350\n",
		"repetido":             "x,X,X,0,pechuga-pollo:60:300|arroz-blanco:30:350\nx,X,X,0,pechuga-pollo:60:300|arroz-blanco:30:350\n",
	}
	for name, rows := range tests {
		if _, err := catalog.ParseDishes([]byte(header+rows), foods); err == nil || !strings.Contains(err.Error(), "plato") {
			t.Errorf("%s: se esperaba un error, obtuve %v", name, err)
		}
	}
}
