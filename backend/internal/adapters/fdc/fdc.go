// Package fdc lee los conjuntos de datos de USDA FoodData Central (JSON) y
// extrae los nutrientes que usa Owli. Los datos de USDA son de dominio público.
package fdc

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/whiteowltech/owli/backend/internal/domain"
)

// IDs de nutrientes de FoodData Central.
const (
	nEnergyKcal     = 1008
	nEnergyAtwaterS = 2048 // Foundation Foods suele traer la energía solo con Atwater
	nEnergyAtwaterG = 2047
	nProtein        = 1003
	nFat            = 1004
	nCarbs          = 1005
	nFiber          = 1079
	nVitaminC       = 1162
	nCalcium        = 1087
	nIron           = 1089
	nMagnesium      = 1090
	nPotassium      = 1092
	nSodium         = 1093
	nZinc           = 1095
	nVitaminB12     = 1178
)

var wanted = map[int]bool{
	nEnergyKcal: true, nEnergyAtwaterS: true, nEnergyAtwaterG: true, nProtein: true, nFat: true, nCarbs: true,
	nFiber: true, nVitaminC: true, nCalcium: true, nIron: true, nMagnesium: true, nPotassium: true,
	nSodium: true, nZinc: true, nVitaminB12: true,
}

// Record es un alimento de USDA con sus nutrientes por 100 g.
type Record struct {
	FdcID       int
	Description string
	Dataset     string // "foundation" o "sr_legacy"
	Nutrients   map[int]float64
}

type rawFood struct {
	FdcID         int    `json:"fdcId"`
	Description   string `json:"description"`
	FoodNutrients []struct {
		Nutrient struct {
			ID int `json:"id"`
		} `json:"nutrient"`
		Amount *float64 `json:"amount"`
	} `json:"foodNutrients"`
}

// Load lee un archivo de FoodData Central en streaming (SR Legacy pesa ~200 MB).
func Load(path, dataset string) (map[int]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("abriendo %s: %w", path, err)
	}
	defer f.Close()
	return decode(f, dataset)
}

func decode(r io.Reader, dataset string) (map[int]Record, error) {
	dec := json.NewDecoder(r)
	// {"FoundationFoods": [ ... ]} o {"SRLegacyFoods": [ ... ]}
	for _, want := range []json.Delim{'{'} {
		if t, err := dec.Token(); err != nil || t != want {
			return nil, fmt.Errorf("formato inesperado al inicio: %v %v", t, err)
		}
	}
	if _, err := dec.Token(); err != nil { // nombre de la lista
		return nil, fmt.Errorf("leyendo la clave principal: %w", err)
	}
	if t, err := dec.Token(); err != nil || t != json.Delim('[') {
		return nil, fmt.Errorf("se esperaba una lista: %v %v", t, err)
	}
	out := map[int]Record{}
	for dec.More() {
		var raw rawFood
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("leyendo alimento: %w", err)
		}
		rec := Record{FdcID: raw.FdcID, Description: raw.Description, Dataset: dataset, Nutrients: map[int]float64{}}
		for _, n := range raw.FoodNutrients {
			if n.Amount != nil && wanted[n.Nutrient.ID] {
				rec.Nutrients[n.Nutrient.ID] = *n.Amount
			}
		}
		out[raw.FdcID] = rec
	}
	return out, nil
}

// Macros devuelve los macros por 100 g. Falla si falta algún macro esencial.
func (r Record) Macros() (domain.Macros, error) {
	kcal, ok := r.Nutrients[nEnergyKcal]
	if !ok {
		if kcal, ok = r.Nutrients[nEnergyAtwaterS]; !ok {
			kcal, ok = r.Nutrients[nEnergyAtwaterG]
		}
	}
	p, okP := r.Nutrients[nProtein]
	f, okF := r.Nutrients[nFat]
	c, okC := r.Nutrients[nCarbs]
	if !ok || !okP || !okF || !okC {
		return domain.Macros{}, fmt.Errorf("fdc %d (%s): faltan macros (energía %v, proteína %v, grasa %v, carbos %v)", r.FdcID, r.Description, ok, okP, okF, okC)
	}
	m := domain.Macros{Calories: kcal, ProteinG: p, FatG: f, CarbsG: c, FiberG: r.Nutrients[nFiber]}
	if m.FiberG > m.CarbsG { // redondeos de USDA en alimentos con casi nada de carbohidratos
		m.FiberG = m.CarbsG
	}
	return m, m.Validate()
}

// Micros devuelve los micros por 100 g; los ausentes quedan en nil (sin dato).
func (r Record) Micros() domain.Micros {
	get := func(id int) *float64 {
		if v, ok := r.Nutrients[id]; ok {
			return &v
		}
		return nil
	}
	return domain.Micros{
		VitaminCMg: get(nVitaminC), CalciumMg: get(nCalcium), IronMg: get(nIron), MagnesiumMg: get(nMagnesium),
		PotassiumMg: get(nPotassium), SodiumMg: get(nSodium), ZincMg: get(nZinc), VitaminB12Ug: get(nVitaminB12),
	}
}

// Search busca descripciones que contengan todas las palabras. Prioriza
// Foundation Foods (más preciso) y descripciones cortas (más genéricas).
func Search(db map[int]Record, query string, limit int) []Record {
	words := strings.Fields(strings.ToLower(query))
	var hits []Record
	for _, r := range db {
		d := strings.ToLower(r.Description)
		match := true
		for _, w := range words {
			if !strings.Contains(d, w) {
				match = false
				break
			}
		}
		if match {
			hits = append(hits, r)
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Dataset != hits[j].Dataset {
			return hits[i].Dataset == "foundation"
		}
		if len(hits[i].Description) != len(hits[j].Description) {
			return len(hits[i].Description) < len(hits[j].Description)
		}
		return hits[i].FdcID < hits[j].FdcID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}
