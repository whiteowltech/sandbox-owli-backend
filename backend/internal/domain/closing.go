package domain

import (
	"math"
	"sort"
	"time"
)

// Meal Closing Engine (CLAUDE.md): calcula lo que falta para la próxima comida y
// sugiere platos concretos que lo cubren, respetando la dieta y las restricciones.
// Los platos son plantillas curadas ("pollo con arroz y palta"); los gramos de cada
// ingrediente se resuelven para lo que falta. Combinar ingredientes al azar da
// platos que cuadran en números pero nadie comería (aprendido en el prototipo v5).

// Un plato "cierra" con las mismas tolerancias que el día (mood.go):
// ClosingCalorieTolerance (±5 % de calorías) y ClosingProteinToleranceG (±5 g de proteína).
const (
	// MaxSuggestions: cuántos platos se sugieren como mucho.
	MaxSuggestions = 10
	// maxPerProtein: variedad, que no salgan 10 variantes de pollo.
	maxPerProtein = 2
	// MinClosingKcal: por debajo de esto el día está cerrado y no se sugiere nada.
	MinClosingKcal = 100
	// MealGap separa dos comidas: anotaciones a menos de 90 min son la misma comida.
	MealGap = 90 * time.Minute
	// ketoMaxNetCarbsPer100g: en keto se descartan los platos con ingredientes más
	// cargados que esto (arroz, papa, pan); las verduras y la palta quedan.
	ketoMaxNetCarbsPer100g = 10.0
	// breakfastPenalty: al desayuno no se sugiere un bife, y a la cena no un yogur con avena;
	// no se excluyen, solo bajan en el orden.
	breakfastPenalty = 0.3
)

// DishComponent es un ingrediente de una plantilla con su porción mínima y máxima
// (una cucharada de aceite no puede volverse 200 g aunque las cuentas lo pidan).
type DishComponent struct {
	Food       Food
	MinG, MaxG float64
}

// DishTemplate es un plato curado. Components[0] es la fuente de proteína; con tres
// componentes se resuelven proteína, carbos y grasa; con dos, proteína y calorías.
type DishTemplate struct {
	ID         string
	NameES     string
	NameEN     string
	Breakfast  bool // plato de desayuno o merienda
	Components []DishComponent
}

// DishPortion es un ingrediente con los gramos resueltos.
type DishPortion struct {
	Food  Food
	Grams float64
}

// DishSuggestion es un plato listo para anotar.
type DishSuggestion struct {
	ID       string
	NameES   string
	NameEN   string
	Portions []DishPortion
	Macros   Macros
	// Exact: cierra lo que falta dentro de la tolerancia (±5 % kcal, ±5 g de proteína).
	Exact bool
	score float64
}

// ClosingRequest es lo que necesita el motor para sugerir.
type ClosingRequest struct {
	Need         Macros // lo que falta para esta comida (en keto, CarbsG son carbos netos)
	Diet         DietType
	Restrictions []Restriction
	Breakfast    bool // es la hora del desayuno en la zona horaria del usuario
}

// MealsLeft cuenta las comidas que quedan: las configuradas (1 a 5) menos las ya
// hechas. Las anotaciones a menos de MealGap entre sí cuentan como una sola comida.
// Siempre queda al menos una: la que se está por hacer.
func MealsLeft(mealsPerDay int, eatenAt []time.Time) int {
	times := append([]time.Time(nil), eatenAt...)
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	meals := 0
	for i, t := range times {
		if i == 0 || t.Sub(times[i-1]) >= MealGap {
			meals++
		}
	}
	return max(1, mealsPerDay-meals)
}

// MealNeed reparte lo que falta del día entre las comidas que quedan. Nada es
// negativo: si un macro ya se cubrió, esta comida no lo necesita.
func MealNeed(dailyKcal float64, targets MacroTargets, eaten Macros, mealsLeft int) Macros {
	n := float64(max(1, mealsLeft))
	carbsEaten := eaten.CarbsG
	if targets.NetCarbs {
		carbsEaten = eaten.NetCarbsG()
	}
	return Macros{
		Calories: math.Max(0, dailyKcal-eaten.Calories) / n,
		ProteinG: math.Max(0, targets.ProteinG-eaten.ProteinG) / n,
		CarbsG:   math.Max(0, targets.CarbsG-carbsEaten) / n,
		FatG:     math.Max(0, targets.FatG-eaten.FatG) / n,
	}
}

// SuggestDishes devuelve hasta MaxSuggestions platos, los que mejor cubren lo que
// falta primero. Cada ingrediente pasa por Allows: ninguna sugerencia se salta la
// dieta ni las restricciones.
func SuggestDishes(templates []DishTemplate, req ClosingRequest) []DishSuggestion {
	if req.Need.Calories < MinClosingKcal {
		return nil
	}
	keto := req.Diet == DietKeto
	var out []DishSuggestion
	for _, t := range templates {
		if !fits(t, req, keto) {
			continue
		}
		grams, ok := solveGrams(t.Components, req.Need, keto)
		if !ok {
			continue
		}
		s := DishSuggestion{ID: t.ID, NameES: t.NameES, NameEN: t.NameEN}
		for i, c := range t.Components {
			s.Portions = append(s.Portions, DishPortion{Food: c.Food, Grams: grams[i]})
			s.Macros = s.Macros.Add(c.Food.Per100g.Scale(grams[i] / 100))
		}
		carbs := s.Macros.CarbsG
		if keto {
			carbs = s.Macros.NetCarbsG()
			if carbs > req.Need.CarbsG+ClosingProteinToleranceG {
				continue // no entra en el margen de carbos netos del día
			}
		}
		need := req.Need
		s.Exact = math.Abs(s.Macros.Calories-need.Calories) <= need.Calories*ClosingCalorieTolerance &&
			math.Abs(s.Macros.ProteinG-need.ProteinG) <= ClosingProteinToleranceG
		s.score = math.Abs(s.Macros.Calories-need.Calories)/math.Max(need.Calories, 100) +
			math.Abs(s.Macros.ProteinG-need.ProteinG)/math.Max(need.ProteinG, 10) +
			0.5*(math.Abs(carbs-need.CarbsG)/math.Max(need.CarbsG, 15)+math.Abs(s.Macros.FatG-need.FatG)/math.Max(need.FatG, 8))
		if t.Breakfast != req.Breakfast {
			s.score += breakfastPenalty
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score < out[j].score })

	perProtein := map[FoodID]int{}
	var top []DishSuggestion
	for _, s := range out {
		p := s.Portions[0].Food.ID
		if perProtein[p] >= maxPerProtein {
			continue
		}
		perProtein[p]++
		top = append(top, s)
		if len(top) == MaxSuggestions {
			break
		}
	}
	return top
}

func fits(t DishTemplate, req ClosingRequest, keto bool) bool {
	if len(t.Components) < 2 || len(t.Components) > 3 {
		return false
	}
	for _, c := range t.Components {
		if !Allows(req.Diet, req.Restrictions, c.Food.Traits) {
			return false
		}
		if keto && c.Food.Per100g.NetCarbsG() > ketoMaxNetCarbsPer100g {
			return false
		}
	}
	return true
}

// solveGrams resuelve los gramos de cada ingrediente para lo que falta:
// con 3 ingredientes, el sistema 3×3 de proteína, carbos y grasa; con 2, el de
// proteína y calorías. Después redondea a 5 g y respeta las porciones mín./máx.
// Si la fuente de proteína pide menos que su porción mínima, el plato no sirve.
func solveGrams(cs []DishComponent, need Macros, keto bool) ([]float64, bool) {
	per := func(c DishComponent) Macros { return c.Food.Per100g.Scale(0.01) } // por gramo
	carbs := func(m Macros) float64 {
		if keto {
			return m.NetCarbsG()
		}
		return m.CarbsG
	}
	var g []float64
	if len(cs) == 3 {
		a, b, c := per(cs[0]), per(cs[1]), per(cs[2])
		m := [3][3]float64{
			{a.ProteinG, b.ProteinG, c.ProteinG},
			{carbs(a), carbs(b), carbs(c)},
			{a.FatG, b.FatG, c.FatG},
		}
		rhs := [3]float64{need.ProteinG, need.CarbsG, need.FatG}
		d := det3(m)
		if math.Abs(d) < 1e-9 {
			return nil, false
		}
		for col := range 3 {
			mc := m
			for row := range 3 {
				mc[row][col] = rhs[row]
			}
			g = append(g, det3(mc)/d)
		}
	} else {
		a, b := per(cs[0]), per(cs[1])
		d := a.ProteinG*b.Calories - b.ProteinG*a.Calories
		if math.Abs(d) < 1e-9 {
			return nil, false
		}
		g = []float64{
			(need.ProteinG*b.Calories - b.ProteinG*need.Calories) / d,
			(a.ProteinG*need.Calories - need.ProteinG*a.Calories) / d,
		}
	}
	if g[0] < cs[0].MinG {
		return nil, false
	}
	for i, c := range cs {
		g[i] = math.Min(c.MaxG, math.Max(c.MinG, math.Max(5, math.Round(g[i]/5)*5)))
	}
	return g, true
}

func det3(m [3][3]float64) float64 {
	return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
		m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
		m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
}
