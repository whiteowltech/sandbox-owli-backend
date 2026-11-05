package domain

import "testing"

func TestReviewWeek(t *testing.T) {
	budget := MacroBudget{Calories: 2000, ProteinG: 150}
	day := func(kcal, p float64, entries int, mood OwliMood) WeekDay {
		return WeekDay{Totals: Macros{Calories: kcal, ProteinG: p}, Entries: entries, Mood: mood}
	}
	closed := day(2010, 152, 3, MoodSatisfied)
	short := day(1900, 90, 3, MoodTired)
	over := day(2400, 150, 4, MoodNeutral)
	empty := WeekDay{}
	today := func(d WeekDay) WeekDay { d.InProgress = true; return d }

	tests := []struct {
		name string
		days []WeekDay
		want WeekReview
	}{
		{
			name: "semana completa",
			days: []WeekDay{closed, short, over, closed, empty, closed, today(day(800, 60, 1, MoodEnergetic))},
			want: WeekReview{LoggedDays: 6, ClosedDays: 3, ProteinShortDays: 1, OverDays: 1,
				AvgKcal: (2010*3 + 1900 + 2400) / 5.0, AvgProteinG: (152*3 + 90 + 150) / 5.0, HasAverages: true, Streak: 2},
		},
		{
			name: "hoy sin anotar no corta la racha",
			days: []WeekDay{empty, closed, closed, today(empty)},
			want: WeekReview{LoggedDays: 2, ClosedDays: 2, AvgKcal: 2010, AvgProteinG: 152, HasAverages: true, Streak: 2},
		},
		{
			name: "un día sin anotar no es un día corto",
			days: []WeekDay{empty, empty, today(empty)},
			want: WeekReview{},
		},
		{
			name: "solo hoy anotado: el promedio es el de hoy",
			days: []WeekDay{empty, today(day(900, 70, 2, MoodNeutral))},
			want: WeekReview{LoggedDays: 1, AvgKcal: 900, AvgProteinG: 70, HasAverages: true, Streak: 1},
		},
		{
			name: "hoy cerrado cuenta como cerrado pero no entra en los promedios",
			days: []WeekDay{short, today(closed)},
			want: WeekReview{LoggedDays: 2, ClosedDays: 1, ProteinShortDays: 1, AvgKcal: 1900, AvgProteinG: 90, HasAverages: true, Streak: 2},
		},
	}
	for _, tt := range tests {
		if got := ReviewWeek(tt.days, budget); got != tt.want {
			t.Errorf("%s:\n got %+v\nwant %+v", tt.name, got, tt.want)
		}
	}
}
