package domain

import "fmt"

// DietType es el perfil dietético elegido en el paso 3 del onboarding.
type DietType string

const (
	DietCarnivore   DietType = "carnivore"   // solo origen animal
	DietKeto        DietType = "keto"        // < 25 g de carbohidratos netos al día
	DietVegan       DietType = "vegan"       // nada de origen animal
	DietVegetarian  DietType = "vegetarian"  // sin carne ni pescado
	DietOmnivore    DietType = "omnivore"    // balanceado, sin exclusiones
	DietPescetarian DietType = "pescetarian" // vegetariano + pescado y marisco
	DietPaleo       DietType = "paleo"       // sin cereales, legumbres, lácteos ni azúcar añadido
)

// KetoNetCarbLimitG es el límite diario de carbohidratos netos en Keto
// (estrictamente menor que este valor).
const KetoNetCarbLimitG = 25.0

// ParseDiet valida un identificador de dieta.
func ParseDiet(s string) (DietType, error) {
	switch d := DietType(s); d {
	case DietCarnivore, DietKeto, DietVegan, DietVegetarian, DietOmnivore, DietPescetarian, DietPaleo:
		return d, nil
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidDiet, s)
}

// FoodTraits son los atributos de un alimento de la base nutricional que
// determinan su compatibilidad con cada dieta.
type FoodTraits struct {
	AnimalOrigin bool // carne, pescado, huevo, lácteo, miel, etc.
	PlantOrigin  bool // cualquier ingrediente vegetal (incluye condimentos, aceites vegetales y café)
	ContainsMeat bool // carne o vísceras de animal terrestre
	ContainsFish bool // pescado o marisco

	ContainsGrain      bool // cereales y pseudocereales (trigo, arroz, maíz, avena…)
	ContainsLegume     bool // legumbres, incluidos soja y maní
	ContainsDairy      bool // leche y derivados
	ContainsAddedSugar bool // azúcar o jarabes añadidos (no el azúcar propio del alimento)

	// Alérgenos e intolerancias. Solo son fiables si AllergensVerified es true:
	// un false sin verificar significa "no sabemos", no "no contiene".
	ContainsGluten    bool
	ContainsLactose   bool
	AllergensVerified bool
}

// Permits indica si un alimento es compatible con la dieta por su naturaleza.
// Para Keto el criterio es el presupuesto diario de carbohidratos netos, que
// se evalúa con KetoFits; aquí no se excluye ningún alimento por categoría.
//
// Carnívoro estricto excluye todo lo vegetal en vez de exigir origen animal:
// el agua, la sal y los suplementos minerales (magnesio, potasio) no son ni
// animales ni vegetales y forman parte de esta dieta. El café es vegetal y
// queda fuera: "estricto" no admite excepciones.
func (d DietType) Permits(t FoodTraits) bool {
	switch d {
	case DietCarnivore:
		return !t.PlantOrigin
	case DietVegan:
		return !t.AnimalOrigin
	case DietVegetarian:
		return !t.ContainsMeat && !t.ContainsFish
	case DietPescetarian:
		return !t.ContainsMeat
	case DietPaleo:
		return !t.ContainsGrain && !t.ContainsLegume && !t.ContainsDairy && !t.ContainsAddedSugar
	case DietKeto, DietOmnivore:
		return true
	}
	return false
}

// KetoFits indica si añadir una porción mantiene el día por debajo del límite
// de carbohidratos netos de Keto.
func KetoFits(consumedToday, portion Macros) bool {
	return consumedToday.NetCarbsG()+portion.NetCarbsG() < KetoNetCarbLimitG
}

// Restriction es una exclusión por salud (alergia o intolerancia) que se
// combina con cualquier dieta: "vegano + sin gluten" es válido.
type Restriction string

const (
	RestrictionGlutenFree  Restriction = "gluten_free"
	RestrictionLactoseFree Restriction = "lactose_free"
)

// ParseRestriction valida un identificador de restricción.
func ParseRestriction(s string) (Restriction, error) {
	switch r := Restriction(s); r {
	case RestrictionGlutenFree, RestrictionLactoseFree:
		return r, nil
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidRestriction, s)
}

// Permits indica si un alimento es seguro para la restricción.
//
// Regla de seguridad: si la base nutricional no verificó los alérgenos del
// alimento, se trata como no apto. Para un celíaco, "no sabemos" equivale a
// "no". El usuario puede anotarlo a mano igualmente; lo que nunca hace Owli
// es sugerirlo.
func (r Restriction) Permits(t FoodTraits) bool {
	if !t.AllergensVerified {
		return false
	}
	switch r {
	case RestrictionGlutenFree:
		return !t.ContainsGluten
	case RestrictionLactoseFree:
		return !t.ContainsLactose
	}
	return false
}

// Allows combina la dieta y todas las restricciones del usuario. Es el único
// filtro que deben usar las sugerencias (Meal Closing Engine).
func Allows(d DietType, restrictions []Restriction, t FoodTraits) bool {
	if !d.Permits(t) {
		return false
	}
	for _, r := range restrictions {
		if !r.Permits(t) {
			return false
		}
	}
	return true
}
