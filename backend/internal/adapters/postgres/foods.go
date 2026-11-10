package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/whiteowltech/owli/backend/internal/adapters/postgres/db"
	"github.com/whiteowltech/owli/backend/internal/catalog"
	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

// CatalogFood es un alimento listo para importar (lo arma cmd/importfoods).
type CatalogFood struct {
	domain.Food
	Aliases   []string // sinónimos: "palta|aguacate"
	Source    string   // usda_foundation, usda_sr_legacy o custom
	SourceRef string   // fdcId en USDA
}

// SyncCatalog importa el catálogo embebido si su versión cambió desde la última
// vez (se llama al arrancar la API; con el catálogo al día no escribe nada).
func (s *Store) SyncCatalog(ctx context.Context, foods []catalog.Food, version string) (imported bool, err error) {
	// Si cambia cómo se arma search_text, el catálogo se vuelve a importar aunque no cambie.
	version += searchFormat
	var current string
	err = s.pool.QueryRow(ctx, "SELECT value FROM app_meta WHERE key = 'foods_seed'").Scan(&current)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("leyendo versión del catálogo: %w", err)
	}
	if current == version {
		return false, nil
	}
	items := make([]CatalogFood, len(foods))
	for i, c := range foods {
		f, err := c.Domain()
		if err != nil {
			return false, err
		}
		items[i] = CatalogFood{Food: f, Aliases: c.Aliases, Source: c.Source, SourceRef: c.SourceRef}
	}
	if err := s.UpsertFoods(ctx, items); err != nil {
		return false, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO app_meta (key, value) VALUES ('foods_seed', $1)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, version)
	if err != nil {
		return false, fmt.Errorf("guardando versión del catálogo: %w", err)
	}
	return true, nil
}

// searchSep separa los nombres y sinónimos en search_text: la búsqueda puntúa cada
// término por separado (ver queries/foods.sql). searchFormat versiona ese formato.
const (
	searchSep    = " | "
	searchFormat = "+s2"
)

// UpsertFoods importa el catálogo en una sola transacción.
func (s *Store) UpsertFoods(ctx context.Context, foods []CatalogFood) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for _, f := range foods {
			terms := append([]string{f.NameES, f.NameEN}, f.Aliases...)
			for i := range terms {
				terms[i] = domain.NormalizeSearch(terms[i])
			}
			m, mi, t := f.Per100g, f.Micros, f.Traits
			err := q.UpsertFood(ctx, db.UpsertFoodParams{
				ID: string(f.ID), NameEs: f.NameES, NameEn: f.NameEN, SearchText: strings.Join(terms, searchSep),
				Source: f.Source, SourceRef: f.SourceRef,
				Kcal: m.Calories, ProteinG: m.ProteinG, CarbsG: m.CarbsG, FiberG: m.FiberG, FatG: m.FatG,
				VitaminCMg: mi.VitaminCMg, CalciumMg: mi.CalciumMg, IronMg: mi.IronMg, MagnesiumMg: mi.MagnesiumMg,
				PotassiumMg: mi.PotassiumMg, SodiumMg: mi.SodiumMg, ZincMg: mi.ZincMg, VitaminB12Ug: mi.VitaminB12Ug,
				AnimalOrigin: t.AnimalOrigin, PlantOrigin: t.PlantOrigin, ContainsMeat: t.ContainsMeat, ContainsFish: t.ContainsFish,
				ContainsGrain: t.ContainsGrain, ContainsLegume: t.ContainsLegume, ContainsDairy: t.ContainsDairy,
				ContainsAddedSugar: t.ContainsAddedSugar, ContainsGluten: t.ContainsGluten, ContainsLactose: t.ContainsLactose,
				AllergensVerified: t.AllergensVerified,
			})
			if err != nil {
				return fmt.Errorf("importando %s: %w", f.ID, err)
			}
		}
		return nil
	})
}

func toFood(r db.Food) domain.Food {
	f := domain.Food{
		ID: domain.FoodID(r.ID), NameES: r.NameEs, NameEN: r.NameEn,
		Per100g: domain.Macros{Calories: r.Kcal, ProteinG: r.ProteinG, CarbsG: r.CarbsG, FiberG: r.FiberG, FatG: r.FatG},
		Micros: domain.Micros{VitaminCMg: r.VitaminCMg, CalciumMg: r.CalciumMg, IronMg: r.IronMg, MagnesiumMg: r.MagnesiumMg,
			PotassiumMg: r.PotassiumMg, SodiumMg: r.SodiumMg, ZincMg: r.ZincMg, VitaminB12Ug: r.VitaminB12Ug},
		Traits: domain.FoodTraits{AnimalOrigin: r.AnimalOrigin, PlantOrigin: r.PlantOrigin, ContainsMeat: r.ContainsMeat,
			ContainsFish: r.ContainsFish, ContainsGrain: r.ContainsGrain, ContainsLegume: r.ContainsLegume,
			ContainsDairy: r.ContainsDairy, ContainsAddedSugar: r.ContainsAddedSugar, ContainsGluten: r.ContainsGluten,
			ContainsLactose: r.ContainsLactose, AllergensVerified: r.AllergensVerified},
	}
	if r.OwnerUserID != nil {
		f.Owner = domain.UserID(r.OwnerUserID.String())
	}
	return f
}

// Foods implementa ports.FoodRepository (separado de Store para no chocar con
// los métodos Get/Upsert del perfil).
type Foods struct{ s *Store }

func (s *Store) Foods() *Foods { return &Foods{s: s} }

func (f *Foods) Get(ctx context.Context, id domain.FoodID) (domain.Food, error) {
	r, err := f.s.q.GetFood(ctx, string(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Food{}, fmt.Errorf("%w: alimento %q", ports.ErrNotFound, id)
	}
	if err != nil {
		return domain.Food{}, fmt.Errorf("leyendo alimento: %w", err)
	}
	return toFood(r), nil
}

func (f *Foods) Search(ctx context.Context, query string, limit int) ([]domain.Food, error) {
	q := domain.NormalizeSearch(query)
	if q == "" {
		return []domain.Food{}, nil
	}
	rows, err := f.s.q.SearchFoods(ctx, db.SearchFoodsParams{Query: q, MaxResults: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("buscando alimentos: %w", err)
	}
	out := make([]domain.Food, len(rows))
	for i, r := range rows {
		out[i] = toFood(r)
	}
	return out, nil
}

func (f *Foods) Match(ctx context.Context, names []string) (domain.Food, bool, error) {
	queries := make([]string, 0, len(names))
	for _, n := range names {
		if q := domain.NormalizeSearch(n); q != "" {
			queries = append(queries, q)
		}
	}
	if len(queries) == 0 {
		return domain.Food{}, false, nil
	}
	rows, err := f.s.q.MatchFoodCandidates(ctx, queries)
	if err != nil {
		return domain.Food{}, false, fmt.Errorf("cruzando alimento: %w", err)
	}
	// Gana el primer nombre de la IA que tenga un candidato con las mismas palabras.
	for _, q := range queries {
		for _, r := range rows {
			for _, term := range strings.Split(r.SearchText, " | ") {
				if domain.NamesMatch(q, term) {
					return toFood(r), true, nil
				}
			}
		}
	}
	return domain.Food{}, false, nil
}

func (f *Foods) SaveEstimate(ctx context.Context, food domain.Food) (domain.Food, error) {
	if !food.Estimated() {
		return domain.Food{}, fmt.Errorf("guardando estimación: %q no tiene dueño", food.ID)
	}
	owner, err := parseUserID(food.Owner)
	if err != nil {
		return domain.Food{}, err
	}
	m := food.Per100g
	err = f.s.q.InsertEstimatedFood(ctx, db.InsertEstimatedFoodParams{
		ID: string(food.ID), NameEs: food.NameES, NameEn: food.NameEN,
		SearchText:  domain.NormalizeSearch(food.NameES) + " | " + domain.NormalizeSearch(food.NameEN),
		OwnerUserID: &owner,
		Kcal:        m.Calories, ProteinG: m.ProteinG, CarbsG: m.CarbsG, FiberG: m.FiberG, FatG: m.FatG,
	})
	if err != nil {
		return domain.Food{}, fmt.Errorf("guardando estimación: %w", err)
	}
	// Si ya existía, se devuelve el guardado (conserva sus valores).
	return f.Get(ctx, food.ID)
}

// Entries implementa ports.EntryRepository.
type Entries struct{ s *Store }

func (s *Store) Entries() *Entries { return &Entries{s: s} }

func (e *Entries) Insert(ctx context.Context, userID domain.UserID, m domain.MealEntry) (string, error) {
	uid, err := parseUserID(userID)
	if err != nil {
		return "", err
	}
	mi := m.Micros
	id, err := e.s.q.InsertEntry(ctx, db.InsertEntryParams{
		UserID: uid, FoodID: string(m.FoodID), Grams: m.Grams, EatenAt: ts(m.EatenAt), Source: string(m.Source),
		Kcal: m.Macros.Calories, ProteinG: m.Macros.ProteinG, CarbsG: m.Macros.CarbsG, FiberG: m.Macros.FiberG, FatG: m.Macros.FatG,
		VitaminCMg: mi.VitaminCMg, CalciumMg: mi.CalciumMg, IronMg: mi.IronMg, MagnesiumMg: mi.MagnesiumMg,
		PotassiumMg: mi.PotassiumMg, SodiumMg: mi.SodiumMg, ZincMg: mi.ZincMg, VitaminB12Ug: mi.VitaminB12Ug,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ForeignKeyViolation {
		return "", fmt.Errorf("%w: usuario o alimento inexistente", ports.ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("guardando comida: %w", err)
	}
	return id.String(), nil
}

func (e *Entries) Delete(ctx context.Context, userID domain.UserID, entryID string) error {
	uid, err := parseUserID(userID)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entryID)
	if err != nil {
		return fmt.Errorf("%w: comida %q", ports.ErrNotFound, entryID)
	}
	n, err := e.s.q.DeleteEntry(ctx, db.DeleteEntryParams{ID: id, UserID: uid})
	if err != nil {
		return fmt.Errorf("borrando comida: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("%w: comida %q", ports.ErrNotFound, entryID)
	}
	return nil
}

func (e *Entries) List(ctx context.Context, userID domain.UserID, from, to time.Time) ([]ports.LoggedEntry, error) {
	uid, err := parseUserID(userID)
	if err != nil {
		return nil, err
	}
	rows, err := e.s.q.ListEntries(ctx, db.ListEntriesParams{UserID: uid, FromTime: ts(from), ToTime: ts(to)})
	if err != nil {
		return nil, fmt.Errorf("listando comidas: %w", err)
	}
	out := make([]ports.LoggedEntry, len(rows))
	for i, r := range rows {
		out[i] = ports.LoggedEntry{
			MealEntry: domain.MealEntry{
				ID: r.ID.String(), FoodID: domain.FoodID(r.FoodID), Grams: r.Grams, EatenAt: r.EatenAt.Time.UTC(),
				Source: domain.EntrySource(r.Source),
				Macros: domain.Macros{Calories: r.Kcal, ProteinG: r.ProteinG, CarbsG: r.CarbsG, FiberG: r.FiberG, FatG: r.FatG},
				Micros: domain.Micros{VitaminCMg: r.VitaminCMg, CalciumMg: r.CalciumMg, IronMg: r.IronMg, MagnesiumMg: r.MagnesiumMg,
					PotassiumMg: r.PotassiumMg, SodiumMg: r.SodiumMg, ZincMg: r.ZincMg, VitaminB12Ug: r.VitaminB12Ug},
			},
			FoodNameES: r.NameEs, FoodNameEN: r.NameEn,
		}
	}
	return out, nil
}

var (
	_ ports.FoodRepository  = (*Foods)(nil)
	_ ports.EntryRepository = (*Entries)(nil)
)
