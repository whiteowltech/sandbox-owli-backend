package main

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

// Precios USD por 1M de tokens (Google AI Studio y claude.com/pricing, noviembre de 2026).
// Los tokens de razonamiento se facturan como salida.
var pricing = map[string]struct{ In, Out float64 }{
	"gemini-3.1-flash-lite": {0.25, 1.50},
	"gemini-3.5-flash-lite": {0.30, 2.50},
	"gemini-3.8-flash":      {0.75, 3.75},
	"claude-haiku-5-5":      {0.10, 0.50},
}

func costUSD(model string, u ports.TokenUsage) float64 {
	p, ok := pricing[model]
	if !ok {
		return math.NaN()
	}
	return (float64(u.InputTokens)*p.In + float64(u.OutputTokens)*p.Out) / 1e6
}

// massErrorPct es el error relativo de la masa total estimada.
func massErrorPct(rec domain.FoodRecognition, truth float64) float64 {
	return math.Abs(rec.TotalGrams()-truth) / truth * 100
}

// weightedRecall es la fracción de la masa real cubierta por ingredientes que
// la IA identificó. Pondera por gramos: acertar el arroz de 150 g importa más
// que el ajo de 2 g, que es invisible en una foto.
//
// La coincidencia de nombres es aproximada (comparten una palabra relevante):
// "grilled chicken" ↔ "chicken breast" cuenta; "mixed greens" ↔ "lettuce" no.
func weightedRecall(rec domain.FoodRecognition, truth []ingredient) float64 {
	var predicted [][]string
	for _, it := range rec.Items {
		predicted = append(predicted, keywords(it.Name))
	}
	var total, hit float64
	for _, ing := range truth {
		total += ing.Grams
		kw := keywords(ing.Name)
		for _, p := range predicted {
			if overlap(kw, p) {
				hit += ing.Grams
				break
			}
		}
	}
	if total == 0 {
		return 0
	}
	return hit / total * 100
}

var stopwords = map[string]bool{
	"and": true, "with": true, "the": true, "raw": true, "cooked": true, "fresh": true,
	"grilled": true, "roasted": true, "fried": true, "boiled": true, "baked": true,
	"steamed": true, "sliced": true, "chopped": true, "mixed": true, "plain": true,
}

func keywords(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) })
	var out []string
	for _, f := range fields {
		if len(f) < 3 || stopwords[f] {
			continue
		}
		out = append(out, singular(f))
	}
	return out
}

func singular(w string) string {
	switch {
	case strings.HasSuffix(w, "oes"), strings.HasSuffix(w, "ies"):
		return w[:len(w)-2]
	case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss"):
		return w[:len(w)-1]
	}
	return w
}

func overlap(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	idx := int(math.Ceil(p/100*float64(len(s)))) - 1
	return s[max(0, min(idx, len(s)-1))]
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}
