// Package memory implementa los repositorios en memoria, con las mismas reglas
// que documentan los puertos. Se usa en tests; no persiste nada.
package memory

import (
	"bytes"
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

type session struct {
	user    domain.UserID
	family  uuid.UUID
	hash    []byte
	expires time.Time
	used    bool
	revoked bool
}

// Store implementa UserRepository, SessionStore y ProfileRepository.
type Store struct {
	mu         sync.Mutex
	foods      map[domain.FoodID]domain.Food
	entries    map[string]entry
	users      map[domain.UserID]bool
	identities map[string]domain.UserID // provider|subject → usuario
	sessions   []*session
	profiles   map[domain.UserID]domain.UserProfile
}

func NewStore() *Store {
	return &Store{
		users:      map[domain.UserID]bool{},
		identities: map[string]domain.UserID{},
		profiles:   map[domain.UserID]domain.UserProfile{},
		foods:      map[domain.FoodID]domain.Food{},
		entries:    map[string]entry{},
	}
}

type entry struct {
	user domain.UserID
	e    domain.MealEntry
}

// AddFood carga un alimento en el catálogo en memoria.
func (s *Store) AddFood(f domain.Food) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.foods[f.ID] = f
}

// HasUser indica si el usuario existe (para verificar borrados en tests).
func (s *Store) HasUser(id domain.UserID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.users[id]
}

func identityKey(p ports.IdentityProvider, sub string) string { return string(p) + "|" + sub }

func (s *Store) CreateUser(context.Context) (domain.UserID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := domain.UserID(uuid.NewString())
	s.users[id] = true
	return id, nil
}

func (s *Store) FindByIdentity(_ context.Context, p ports.IdentityProvider, sub string) (domain.UserID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.identities[identityKey(p, sub)]; ok {
		return id, nil
	}
	return "", ports.ErrNotFound
}

func (s *Store) LinkIdentity(_ context.Context, userID domain.UserID, e ports.ExternalIdentity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := identityKey(e.Provider, e.Subject)
	if owner, ok := s.identities[k]; ok && owner != userID {
		return ports.ErrIdentityInUse
	}
	s.identities[k] = userID
	return nil
}

func (s *Store) DeleteUser(_ context.Context, userID domain.UserID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, userID)
	delete(s.profiles, userID)
	for id, x := range s.entries {
		if x.user == userID {
			delete(s.entries, id)
		}
	}
	for k, v := range s.identities {
		if v == userID {
			delete(s.identities, k)
		}
	}
	kept := s.sessions[:0]
	for _, x := range s.sessions {
		if x.user != userID {
			kept = append(kept, x)
		}
	}
	s.sessions = kept
	return nil
}

func (s *Store) find(h []byte) *session {
	for _, x := range s.sessions {
		if bytes.Equal(x.hash, h) {
			return x
		}
	}
	return nil
}

func (s *Store) revokeFamily(f uuid.UUID) {
	for _, x := range s.sessions {
		if x.family == f {
			x.revoked = true
		}
	}
}

func (s *Store) Create(_ context.Context, u domain.UserID, h []byte, exp time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = append(s.sessions, &session{user: u, family: uuid.New(), hash: h, expires: exp})
	return nil
}

func (s *Store) Rotate(_ context.Context, oldH, newH []byte, newExp, now time.Time) (domain.UserID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x := s.find(oldH)
	switch {
	case x == nil:
		return "", ports.ErrSessionNotFound
	case x.used || x.revoked:
		s.revokeFamily(x.family)
		return "", ports.ErrSessionReused
	case !now.Before(x.expires):
		return "", ports.ErrSessionExpired
	}
	x.used = true
	s.sessions = append(s.sessions, &session{user: x.user, family: x.family, hash: newH, expires: newExp})
	return x.user, nil
}

func (s *Store) Revoke(_ context.Context, h []byte, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x := s.find(h); x != nil {
		s.revokeFamily(x.family)
	}
	return nil
}

func (s *Store) Get(_ context.Context, u domain.UserID) (domain.UserProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.profiles[u]
	if !ok {
		return domain.UserProfile{}, ports.ErrNotFound
	}
	return p, nil
}

func (s *Store) Upsert(_ context.Context, u domain.UserID, p domain.UserProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.users[u] {
		return ports.ErrNotFound // igual que la clave foránea de Postgres
	}
	s.profiles[u] = p
	return nil
}

// FoodsRepo y EntriesRepo exponen los repositorios de alimentos y comidas
// (los métodos Get/Delete chocarían con los del perfil y los usuarios).
func (s *Store) FoodsRepo() *Foods     { return &Foods{s: s} }
func (s *Store) EntriesRepo() *Entries { return &Entries{s: s} }

type Foods struct{ s *Store }

func (f *Foods) Get(_ context.Context, id domain.FoodID) (domain.Food, error) {
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	food, ok := f.s.foods[id]
	if !ok {
		return domain.Food{}, ports.ErrNotFound
	}
	return food, nil
}

func (f *Foods) Search(_ context.Context, query string, limit int) ([]domain.Food, error) {
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	q := domain.NormalizeSearch(query)
	out := []domain.Food{}
	for _, food := range f.s.foods {
		if strings.Contains(domain.NormalizeSearch(food.NameES+" "+food.NameEN), q) {
			out = append(out, food)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NameES < out[j].NameES })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Match: el primer nombre que coincide con algún alimento (versión simple para pruebas).
func (f *Foods) Match(ctx context.Context, names []string) (domain.Food, bool, error) {
	for _, n := range names {
		if n == "" {
			continue
		}
		if found, _ := f.Search(ctx, n, 1); len(found) > 0 {
			return found[0], true, nil
		}
	}
	return domain.Food{}, false, nil
}

type Entries struct{ s *Store }

func (e *Entries) Insert(_ context.Context, u domain.UserID, m domain.MealEntry) (string, error) {
	e.s.mu.Lock()
	defer e.s.mu.Unlock()
	if !e.s.users[u] {
		return "", ports.ErrNotFound
	}
	if _, ok := e.s.foods[m.FoodID]; !ok {
		return "", ports.ErrNotFound
	}
	m.ID = uuid.NewString()
	e.s.entries[m.ID] = entry{user: u, e: m}
	return m.ID, nil
}

func (e *Entries) Delete(_ context.Context, u domain.UserID, id string) error {
	e.s.mu.Lock()
	defer e.s.mu.Unlock()
	x, ok := e.s.entries[id]
	if !ok || x.user != u {
		return ports.ErrNotFound
	}
	delete(e.s.entries, id)
	return nil
}

func (e *Entries) List(_ context.Context, u domain.UserID, from, to time.Time) ([]ports.LoggedEntry, error) {
	e.s.mu.Lock()
	defer e.s.mu.Unlock()
	out := []ports.LoggedEntry{}
	for _, x := range e.s.entries {
		if x.user == u && !x.e.EatenAt.Before(from) && x.e.EatenAt.Before(to) {
			f := e.s.foods[x.e.FoodID]
			out = append(out, ports.LoggedEntry{MealEntry: x.e, FoodNameES: f.NameES, FoodNameEN: f.NameEN})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EatenAt.Before(out[j].EatenAt) })
	return out, nil
}

var (
	_ ports.FoodRepository    = (*Foods)(nil)
	_ ports.EntryRepository   = (*Entries)(nil)
	_ ports.UserRepository    = (*Store)(nil)
	_ ports.SessionStore      = (*Store)(nil)
	_ ports.ProfileRepository = (*Store)(nil)
)
