package domain

// Micros son los micronutrientes que sigue Owli, por porción o por 100 g.
//
// Regla: nil significa "la base no tiene el dato" y NUNCA se trata como cero
// (ver pendientes.md). Al sumar, un dato faltante deja el total como parcial y
// la UI muestra la cobertura.
type Micros struct {
	VitaminCMg   *float64
	CalciumMg    *float64
	IronMg       *float64
	MagnesiumMg  *float64
	PotassiumMg  *float64
	SodiumMg     *float64
	ZincMg       *float64
	VitaminB12Ug *float64
}

// fields enumera los campos para operar sobre todos de forma uniforme.
func (m *Micros) fields() []**float64 {
	return []**float64{&m.VitaminCMg, &m.CalciumMg, &m.IronMg, &m.MagnesiumMg, &m.PotassiumMg, &m.SodiumMg, &m.ZincMg, &m.VitaminB12Ug}
}

// Scale multiplica los valores conocidos; los faltantes siguen faltando.
func (m Micros) Scale(k float64) Micros {
	out := m
	for _, f := range out.fields() {
		if *f != nil {
			v := **f * k
			*f = &v
		}
	}
	return out
}

// MicroTotals acumula micros de varios alimentos y cuenta cuántos tenían cada dato.
type MicroTotals struct {
	Sum Micros
	// Known[i] es la cantidad de alimentos con dato para el micro i (mismo orden que fields).
	Known [8]int
	Items int
}

// Add suma un alimento. Los datos faltantes no suman cero: solo no cuentan como conocidos.
func (t *MicroTotals) Add(m Micros) {
	t.Items++
	sum := t.Sum.fields()
	for i, f := range m.fields() {
		if *f == nil {
			continue
		}
		t.Known[i]++
		if *sum[i] == nil {
			v := 0.0
			*sum[i] = &v
		}
		**sum[i] += **f
	}
}
