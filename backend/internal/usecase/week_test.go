package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

func TestWeek(t *testing.T) {
	svc, store, u := newDiary(t) // hoy: 3 de noviembre, 20 h en Buenos Aires
	ctx := context.Background()
	if _, err := svc.Week(ctx, u, ""); !errors.Is(err, ports.ErrProfileRequired) {
		t.Fatalf("sin perfil: %v", err)
	}
	saveProfile(t, store, u)

	log := func(utc time.Time, grams float64) {
		t.Helper()
		if _, err := svc.Log(ctx, u, "pechuga-pollo", grams, utc, domain.SourceManual); err != nil {
			t.Fatal(err)
		}
	}
	// 2 de noviembre 02:00 UTC = 1 de noviembre 23:00 en Buenos Aires: cuenta el día 1.
	log(time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC), 200)
	log(time.Date(2026, 11, 1, 15, 0, 0, 0, time.UTC), 100)
	log(time.Date(2026, 11, 3, 15, 0, 0, 0, time.UTC), 150) // hoy

	w, err := svc.Week(ctx, u, "")
	if err != nil {
		t.Fatal(err)
	}
	if w.Start != "2026-10-28" || w.End != "2026-11-03" || w.Today != "2026-11-03" || len(w.Days) != 7 {
		t.Fatalf("semana = %s a %s (%d días)", w.Start, w.End, len(w.Days))
	}
	nov1, nov2, nov3 := w.Days[4], w.Days[5], w.Days[6]
	if nov1.Date != "2026-11-01" || nov1.Entries != 2 || nov1.Totals.Calories != 165*3 {
		t.Errorf("1 de noviembre = %+v", nov1)
	}
	if nov2.Entries != 0 || nov3.Entries != 1 || !nov3.InProgress || nov2.InProgress {
		t.Errorf("2 y 3 de noviembre = %+v / %+v", nov2, nov3)
	}
	if w.Review.LoggedDays != 2 || w.Review.Streak != 1 || w.Plan.DailyTarget == 0 {
		t.Errorf("resumen = %+v", w.Review)
	}

	// Semana anterior.
	prev, err := svc.Week(ctx, u, "2026-10-27")
	if err != nil || prev.Start != "2026-10-21" || prev.Review.LoggedDays != 0 {
		t.Errorf("semana anterior = %+v, %v", prev, err)
	}
	// Futuro o fecha inválida: error de validación.
	for _, bad := range []string{"2026-11-04", "3/11/2026"} {
		if _, err := svc.Week(ctx, u, bad); !errors.Is(err, domain.ErrInvalidProfile) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}
