package catalog

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/whiteowltech/owli/backend/internal/domain"
)

// ParseDishes lee los platos curados (data/dishes_catalog.csv) y los resuelve con
// los alimentos del catálogo. Falla si un plato nombra un alimento que no existe o
// tiene porciones inválidas: mejor no arrancar que sugerir un plato roto.
// Columnas: id, name_es, name_en, kind (meal/breakfast/side), components ("food_id:min:max|…").
func ParseDishes(raw []byte, foods []Food) ([]domain.DishTemplate, error) {
	byID := make(map[string]domain.Food, len(foods))
	for _, f := range foods {
		d, err := f.Domain()
		if err != nil {
			return nil, err
		}
		byID[f.ID] = d
	}

	r := csv.NewReader(bytes.NewReader(raw))
	r.Comment = '#'
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("platos: leyendo encabezado: %w", err)
	}
	if strings.Join(header, ",") != "id,name_es,name_en,kind,components" {
		return nil, fmt.Errorf("platos: encabezado inesperado: %v", header)
	}
	seen := map[string]bool{}
	var out []domain.DishTemplate
	var problems []string
	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("platos: %w", err)
		}
		t := domain.DishTemplate{ID: row[0], NameES: row[1], NameEN: row[2], Breakfast: row[3] == "breakfast", Side: row[3] == "side"}
		fail := func(msg string, args ...any) {
			problems = append(problems, fmt.Sprintf("plato %s: ", t.ID)+fmt.Sprintf(msg, args...))
		}
		switch {
		case seen[t.ID]:
			fail("id repetido")
			continue
		case t.NameES == "" || t.NameEN == "":
			fail("falta el nombre")
			continue
		case row[3] != "meal" && row[3] != "breakfast" && row[3] != "side":
			fail("kind debe ser meal, breakfast o side")
			continue
		}
		seen[t.ID] = true
		ok := true
		for _, part := range strings.Split(row[4], "|") {
			fields := strings.Split(part, ":")
			if len(fields) != 3 {
				fail("componente %q: se espera food_id:min:max", part)
				ok = false
				break
			}
			food, found := byID[fields[0]]
			minG, errMin := strconv.ParseFloat(fields[1], 64)
			maxG, errMax := strconv.ParseFloat(fields[2], 64)
			switch {
			case !found:
				fail("el alimento %q no está en el catálogo", fields[0])
			case errMin != nil || errMax != nil || minG <= 0 || maxG < minG || maxG > domain.MaxEntryGrams:
				fail("porciones inválidas en %q", part)
			default:
				t.Components = append(t.Components, domain.DishComponent{Food: food, MinG: minG, MaxG: maxG})
				continue
			}
			ok = false
			break
		}
		if !ok {
			continue
		}
		if n := len(t.Components); t.Side && n > 2 {
			fail("acompañamiento con %d ingredientes (se resuelven 1 o 2)", n)
			continue
		} else if !t.Side && (n < 2 || n > 3) {
			fail("tiene %d ingredientes (se resuelven 2 o 3)", n)
			continue
		}
		out = append(out, t)
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("el catálogo de platos tiene %d problemas:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	return out, nil
}
