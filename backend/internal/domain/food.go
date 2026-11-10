package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
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
	// Owner es el usuario dueño de un alimento propio (vacío en el catálogo). Un
	// alimento propio es privado: solo su dueño lo ve, lo busca y lo anota.
	Owner  UserID
	Origin FoodOrigin
	// ServingGrams es el peso de una porción de una receta propia (0 = sin dato):
	// "comí dos waffles" son dos porciones.
	ServingGrams float64
}

// FoodOrigin dice de dónde salen los valores de un alimento.
type FoodOrigin string

const (
	OriginCatalog    FoodOrigin = ""            // catálogo curado (USDA o recetas de Owli)
	OriginAIEstimate FoodOrigin = "ai_estimate" // propio: lo estimó la IA
	OriginUserRecipe FoodOrigin = "user_recipe" // propio: receta del usuario, calculada con sus ingredientes
)

// Estimated indica que los valores los estimó la IA (no salen de la base).
func (f Food) Estimated() bool { return f.Origin == OriginAIEstimate }

// IsRecipe indica que es una receta propia del usuario.
func (f Food) IsRecipe() bool { return f.Origin == OriginUserRecipe }

// VisibleTo indica si el usuario puede ver y anotar el alimento.
func (f Food) VisibleTo(u UserID) bool { return f.Owner == "" || f.Owner == u }

// maxEstimatedNameRunes acota el nombre que llega de la IA.
const maxEstimatedNameRunes = 80

// NewEstimatedFood arma el alimento privado de un usuario a partir de lo que
// identificó y estimó la IA. El ID sale del dueño y del nombre normalizado, así que
// el mismo alimento ("pancake proteico") se reutiliza la próxima vez con los mismos
// valores. No tiene micros (la IA no los estima) ni alérgenos verificados: para las
// restricciones, "no sabemos" equivale a "no apto".
func NewEstimatedFood(owner UserID, nameES, nameEN string, e NutrientEstimate) (Food, error) {
	if owner == "" {
		return Food{}, fmt.Errorf("%w: un alimento estimado necesita dueño", ErrInvalidMacros)
	}
	nameES, nameEN = clipName(nameES), clipName(nameEN)
	if nameES == "" {
		nameES = nameEN
	}
	if nameEN == "" {
		nameEN = nameES
	}
	id, err := ownFoodID("ai", owner, nameES)
	if err != nil {
		return Food{}, err
	}
	per100, err := e.Per100g()
	if err != nil {
		return Food{}, err
	}
	return Food{ID: id, NameES: nameES, NameEN: nameEN, Per100g: per100, Owner: owner, Origin: OriginAIEstimate}, nil
}

// ownFoodID: el ID de un alimento propio sale del dueño y del nombre normalizado,
// así que el mismo nombre es siempre el mismo alimento de ese usuario.
func ownFoodID(prefix string, owner UserID, name string) (FoodID, error) {
	key := NormalizeSearch(name)
	if key == "" {
		return "", fmt.Errorf("%w: alimento propio sin nombre", ErrInvalidMacros)
	}
	sum := sha256.Sum256([]byte(string(owner) + "|" + key))
	return FoodID(prefix + "-" + hex.EncodeToString(sum[:10])), nil
}

// RecipeIngredient es un ingrediente de una receta propia: un alimento que el
// usuario puede ver (del catálogo o propio) y los gramos que lleva.
type RecipeIngredient struct {
	Food  Food
	Grams float64
}

// MaxRecipeServings acota las porciones de una receta propia.
const MaxRecipeServings = 50

// NewUserRecipe arma la receta propia de un usuario ("Waffles proteicos") con sus
// ingredientes. Los valores por 100 g salen de ComposeRecipe con el peso de los
// ingredientes como peso final (no sabemos cuánta agua pierde al cocinarse). Con
// servings > 0, una porción pesa el total dividido por las porciones. El ID sale
// del dueño y del nombre: volver a dictar la receta la actualiza.
func NewUserRecipe(owner UserID, name string, servings float64, ingredients []RecipeIngredient) (Food, error) {
	if owner == "" {
		return Food{}, fmt.Errorf("%w: una receta propia necesita dueño", ErrInvalidMacros)
	}
	if servings < 0 || servings > MaxRecipeServings {
		return Food{}, fmt.Errorf("%w: %.1f porciones (0 a %d)", ErrInvalidMacros, servings, MaxRecipeServings)
	}
	name = clipName(name)
	id, err := ownFoodID("rc", owner, name)
	if err != nil {
		return Food{}, err
	}
	parts := make([]RecipePart, 0, len(ingredients))
	total := 0.0
	for _, in := range ingredients {
		if !in.Food.VisibleTo(owner) {
			return Food{}, fmt.Errorf("%w: ingrediente %q", ErrInvalidMacros, in.Food.ID)
		}
		if in.Grams > MaxEntryGrams {
			return Food{}, fmt.Errorf("%w: ingrediente con %.0f g", ErrInvalidMacros, in.Grams)
		}
		parts = append(parts, RecipePart{Per100g: in.Food.Per100g, Micros: in.Food.Micros, Grams: in.Grams})
		total += in.Grams
	}
	per100, micros, err := ComposeRecipe(parts, 0)
	if err != nil {
		return Food{}, err
	}
	f := Food{ID: id, NameES: name, NameEN: name, Per100g: per100, Micros: micros, Owner: owner, Origin: OriginUserRecipe}
	if servings > 0 {
		f.ServingGrams = math.Round(total/servings*10) / 10
	}
	return f, nil
}

// RecipeTotalGrams es el peso de toda la receta (la suma de sus ingredientes).
func RecipeTotalGrams(ingredients []RecipeIngredient) float64 {
	t := 0.0
	for _, in := range ingredients {
		t += in.Grams
	}
	return t
}

func clipName(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxEstimatedNameRunes {
		s = string(r[:maxEstimatedNameRunes])
	}
	// "pancake proteico" → "Pancake proteico": como los nombres del catálogo.
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
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

// matchFiller son palabras que no cambian qué alimento es: "cooked white rice" es
// el mismo que "white rice". Las preparaciones que sí cambian los valores (frito,
// guisado, crudo, a la parrilla) no están: "stewed beef" no es "beef steak".
var matchFiller = map[string]bool{
	"cooked": true, "cocido": true, "cocida": true, "cocidos": true, "cocidas": true,
	"de": true, "del": true, "con": true, "el": true, "la": true, "los": true, "las": true, "al": true, "a": true, "y": true,
	"the": true, "of": true, "with": true, "and": true,
}

// NamesMatch indica si dos nombres de alimento son el mismo: las mismas palabras,
// sin acentos, sin mayúsculas, sin plurales y sin las de matchFiller. Es la regla
// para cruzar lo que identificó la IA con el catálogo: una coincidencia parecida
// (que comparte una palabra) reemplazaba el alimento por otro.
func NamesMatch(a, b string) bool {
	wa, wb := matchWords(a), matchWords(b)
	if len(wa) == 0 || len(wa) != len(wb) {
		return false
	}
	for w := range wa {
		if !wb[w] {
			return false
		}
	}
	return true
}

func matchWords(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(NormalizeSearch(s)) {
		if matchFiller[w] {
			continue
		}
		out[singular(w)] = true
	}
	return out
}

// singular quita el plural común en español e inglés ("papas", "potatoes", "huevos").
func singular(w string) string {
	switch {
	case len(w) > 4 && strings.HasSuffix(w, "oes"): // potatoes, tomatoes
		return w[:len(w)-2]
	case len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss"):
		return w[:len(w)-1]
	}
	return w
}
