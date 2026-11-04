package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// FoodID identifica un alimento del catálogo (slug estable, p. ej. "pechuga-pollo-plancha").
type FoodID string

// Food es un alimento de la base nutricional. Todos los valores son por 100 g.
type Food struct {
	ID      FoodID
	NameES  string
	NameEN  string
	Per100g Macros
	Micros  Micros // por 100 g; campos nil = sin dato
	Traits  FoodTraits
}

// MaxEntryGrams acota una porción registrada (evita errores de tipeo como 25000 g).
const MaxEntryGrams = 3000

// EntrySource indica cómo se registró una comida.
type EntrySource string

const (
	SourceManual     EntrySource = "manual"
	SourcePhoto      EntrySource = "photo"
	SourceVoice      EntrySource = "voice"
	SourceSuggestion EntrySource = "suggestion"
)

// MealEntry es un alimento registrado. Guarda una copia de los macros y micros
// calculados al registrarlo: si la base nutricional se corrige después, el
// historial del usuario no cambia solo.
type MealEntry struct {
	ID      string
	FoodID  FoodID
	Grams   float64
	EatenAt time.Time // en UTC
	Source  EntrySource
	Macros  Macros
	Micros  Micros
}

// NewEntry calcula la porción a partir del alimento y los gramos.
func NewEntry(f Food, grams float64, eatenAt time.Time, src EntrySource) (MealEntry, error) {
	if grams <= 0 || grams > MaxEntryGrams {
		return MealEntry{}, fmt.Errorf("%w: %.1f g fuera de rango (0 a %d)", ErrInvalidMacros, grams, MaxEntryGrams)
	}
	switch src {
	case SourceManual, SourcePhoto, SourceVoice, SourceSuggestion:
	default:
		return MealEntry{}, fmt.Errorf("%w: origen %q desconocido", ErrInvalidMacros, src)
	}
	m, err := ForGrams(f.Per100g, grams)
	if err != nil {
		return MealEntry{}, err
	}
	return MealEntry{FoodID: f.ID, Grams: grams, EatenAt: eatenAt.UTC(), Source: src, Macros: m, Micros: f.Micros.Scale(grams / 100)}, nil
}

// DayBounds devuelve el inicio y el fin (exclusivo) del día civil date
// ("2006-01-02") en la zona horaria del usuario, expresados en UTC. Los totales
// diarios se cortan así, nunca con la hora del servidor (regla de CLAUDE.md).
// Con cambio de horario un día puede durar 23 o 25 horas.
func DayBounds(date, timezone string) (start, end time.Time, err error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: zona horaria %q", ErrInvalidProfile, timezone)
	}
	d, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: fecha %q (se espera AAAA-MM-DD)", ErrInvalidProfile, date)
	}
	return d.UTC(), d.AddDate(0, 0, 1).UTC(), nil
}

// Today es la fecha civil actual en la zona horaria del usuario.
func Today(now time.Time, timezone string) (string, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return "", fmt.Errorf("%w: zona horaria %q", ErrInvalidProfile, timezone)
	}
	return now.In(loc).Format("2006-01-02"), nil
}

// DayTotals suma los macros y micros de las comidas de un día.
func DayTotals(entries []MealEntry) (Macros, MicroTotals) {
	var total Macros
	var micros MicroTotals
	for _, e := range entries {
		total = total.Add(e.Macros)
		micros.Add(e.Micros)
	}
	return total, micros
}

// searchFolds reemplaza letras acentuadas para buscar "limon" y encontrar "limón".
var searchFolds = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	"à", "a", "è", "e", "ì", "i", "ò", "o", "ù", "u", "â", "a", "ê", "e", "î", "i", "ô", "o", "û", "u", "ç", "c",
)

// NormalizeSearch prepara un texto para la búsqueda: minúsculas, sin acentos ni
// signos, con espacios simples. Se aplica igual al guardar y al buscar.
func NormalizeSearch(s string) string {
	s = searchFolds.Replace(strings.ToLower(s))
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space && b.Len() > 0 {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}
