package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

func ptr(v float64) *float64 { return &v }

var pollo = Food{
	ID: "pechuga-pollo", NameES: "Pechuga de pollo asada", NameEN: "Roasted chicken breast",
	Per100g: Macros{Calories: 165, ProteinG: 31, FatG: 3.6},
	Micros:  Micros{PotassiumMg: ptr(256), SodiumMg: ptr(74)}, // el resto: sin dato
}

func TestNewEntry(t *testing.T) {
	at := time.Date(2026, 11, 3, 15, 0, 0, 0, time.FixedZone("ART", -3*3600))
	e, err := NewEntry(pollo, 150, at, SourceManual)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	if math.Abs(e.Macros.Calories-247.5) > 1e-9 || math.Abs(e.Macros.ProteinG-46.5) > 1e-9 {
		t.Errorf("macros = %+v", e.Macros)
	}
	if *e.Micros.PotassiumMg != 384 || e.Micros.IronMg != nil {
		t.Errorf("micros: potasio %v, hierro %v (sin dato debe seguir sin dato)", *e.Micros.PotassiumMg, e.Micros.IronMg)
	}
	if e.EatenAt.Location() != time.UTC || !e.EatenAt.Equal(at) {
		t.Errorf("la hora se guarda en UTC: %v", e.EatenAt)
	}
	if *pollo.Micros.PotassiumMg != 256 {
		t.Error("escalar una porción no debe modificar el alimento original")
	}

	for _, g := range []float64{0, -5, 3001} {
		if _, err := NewEntry(pollo, g, at, SourceManual); !errors.Is(err, ErrInvalidMacros) {
			t.Errorf("%v g = %v, se esperaba ErrInvalidMacros", g, err)
		}
	}
	if _, err := NewEntry(pollo, 100, at, "telepatía"); !errors.Is(err, ErrInvalidMacros) {
		t.Errorf("origen desconocido = %v", err)
	}
}

func TestDayBounds(t *testing.T) {
	tests := []struct {
		name, date, tz string
		start, end     string // UTC
	}{
		{"Buenos Aires (UTC−3)", "2026-11-03", "America/Argentina/Buenos_Aires", "2026-11-03T03:00:00Z", "2026-11-04T03:00:00Z"},
		{"Madrid en invierno (UTC+1)", "2026-11-03", "Europe/Madrid", "2026-11-02T23:00:00Z", "2026-11-03T23:00:00Z"},
		{"Nueva York, día de 23 h (empieza el horario de verano)", "2026-03-08", "America/New_York", "2026-03-08T05:00:00Z", "2026-03-09T04:00:00Z"},
		{"Nueva York, día de 25 h (termina el horario de verano)", "2026-11-01", "America/New_York", "2026-11-01T04:00:00Z", "2026-11-02T05:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := DayBounds(tt.date, tt.tz)
			if err != nil {
				t.Fatal(err)
			}
			if got := start.Format(time.RFC3339); got != tt.start {
				t.Errorf("inicio = %s, se esperaba %s", got, tt.start)
			}
			if got := end.Format(time.RFC3339); got != tt.end {
				t.Errorf("fin = %s, se esperaba %s", got, tt.end)
			}
		})
	}
	if _, _, err := DayBounds("03/11/2026", "UTC"); !errors.Is(err, ErrInvalidProfile) {
		t.Errorf("formato de fecha inválido = %v", err)
	}
	if _, _, err := DayBounds("2026-11-03", "Marte/Olympus"); !errors.Is(err, ErrInvalidProfile) {
		t.Errorf("zona inválida = %v", err)
	}
}

func TestToday(t *testing.T) {
	// 01:30 UTC del 4 de noviembre todavía es 3 de noviembre en Buenos Aires.
	now := time.Date(2026, 11, 4, 1, 30, 0, 0, time.UTC)
	if got, _ := Today(now, "America/Argentina/Buenos_Aires"); got != "2026-11-03" {
		t.Errorf("hoy en Buenos Aires = %s", got)
	}
	if got, _ := Today(now, "Asia/Tokyo"); got != "2026-11-04" {
		t.Errorf("hoy en Tokio = %s", got)
	}
}

func TestDayTotalsKeepsMissingMicrosApart(t *testing.T) {
	pan := Food{ID: "pan", Per100g: Macros{Calories: 266, ProteinG: 9, CarbsG: 49, FatG: 3}} // sin micros
	a, _ := NewEntry(pollo, 100, time.Now(), SourceManual)
	b, _ := NewEntry(pan, 50, time.Now(), SourceManual)
	total, micros := DayTotals([]MealEntry{a, b})
	if math.Abs(total.Calories-298) > 1e-9 {
		t.Errorf("kcal = %v", total.Calories)
	}
	if micros.Items != 2 || micros.Known[4] != 1 || *micros.Sum.PotassiumMg != 256 {
		t.Errorf("potasio: %d de %d alimentos con dato, total %v", micros.Known[4], micros.Items, *micros.Sum.PotassiumMg)
	}
	if micros.Sum.IronMg != nil {
		t.Error("sin ningún dato de hierro, el total es 'sin dato', no cero")
	}
}

func TestNormalizeSearch(t *testing.T) {
	tests := map[string]string{
		"Limón":                   "limon",
		"  JAMÓN   cocido ":       "jamon cocido",
		"Ñandú":                   "nandu",
		"Pan (lactal), integral!": "pan lactal integral",
		"Agüita":                  "aguita",
		"":                        "",
	}
	for in, want := range tests {
		if got := NormalizeSearch(in); got != want {
			t.Errorf("NormalizeSearch(%q) = %q, se esperaba %q", in, got, want)
		}
	}
}
