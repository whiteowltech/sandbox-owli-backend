package domain

// WeekDay es un día en el resumen semanal.
type WeekDay struct {
	Date    string // AAAA-MM-DD en la zona horaria del usuario
	Totals  Macros
	Entries int
	// InProgress: es hoy; el día no terminó y no cuenta como "corto" ni "pasado".
	InProgress bool
	Mood       OwliMood
}

// Logged indica que el usuario anotó algo ese día.
func (d WeekDay) Logged() bool { return d.Entries > 0 }

// Closed: el día quedó dentro de las tolerancias de calorías y proteína (el mismo
// criterio que el estado "satisfecho" de Owli).
func (d WeekDay) Closed() bool { return d.Mood == MoodSatisfied }

// WeekReview resume una semana para la pantalla "Tu semana".
type WeekReview struct {
	LoggedDays int // días con algo anotado
	ClosedDays int // días cerrados (calorías ±5 % y proteína a 5 g o más)
	// ProteinShortDays y OverDays solo cuentan días terminados y anotados: un día sin
	// anotar no es un día "corto", es un día sin datos.
	ProteinShortDays int
	OverDays         int // más del 5 % por encima de las calorías
	// Promedios de los días terminados con anotaciones; si no hay ninguno, del día en curso.
	// HasAverages es false si no hay ningún día anotado.
	AvgKcal     float64
	AvgProteinG float64
	HasAverages bool
	// Streak: días seguidos anotando hasta hoy (si hoy todavía no se anotó, hasta ayer).
	Streak int
}

// ReviewWeek resume los días, en orden cronológico (el último es el más reciente).
func ReviewWeek(days []WeekDay, budget MacroBudget) WeekReview {
	var r WeekReview
	var sum, inProgress Macros
	completed := 0
	for _, d := range days {
		if !d.Logged() {
			continue
		}
		r.LoggedDays++
		if d.Closed() {
			r.ClosedDays++
		}
		if d.InProgress {
			inProgress = d.Totals
			continue
		}
		completed++
		sum = sum.Add(d.Totals)
		if !d.Closed() && d.Totals.ProteinG < budget.ProteinG-ClosingProteinToleranceG {
			r.ProteinShortDays++
		}
		if budget.Calories > 0 && d.Totals.Calories > budget.Calories*(1+ClosingCalorieTolerance) {
			r.OverDays++
		}
	}
	switch {
	case completed > 0:
		r.AvgKcal, r.AvgProteinG, r.HasAverages = sum.Calories/float64(completed), sum.ProteinG/float64(completed), true
	case r.LoggedDays > 0:
		r.AvgKcal, r.AvgProteinG, r.HasAverages = inProgress.Calories, inProgress.ProteinG, true
	}

	i := len(days) - 1
	if i >= 0 && days[i].InProgress && !days[i].Logged() {
		i-- // hoy todavía no anotó: la racha sigue viva hasta ayer
	}
	for ; i >= 0 && days[i].Logged(); i-- {
		r.Streak++
	}
	return r
}
