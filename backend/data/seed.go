// Package data contiene el catálogo de alimentos ya resuelto (foods_seed.json),
// embebido en el binario. Lo genera cmd/importfoods a partir de USDA y del
// catálogo curado; la API lo carga al arrancar si cambió.
package data

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

//go:embed foods_seed.json
var FoodsSeed []byte

// FoodsSeedVersion identifica el contenido del catálogo (hash). Si no cambió,
// la API no vuelve a importarlo al arrancar.
func FoodsSeedVersion() string {
	sum := sha256.Sum256(FoodsSeed)
	return hex.EncodeToString(sum[:8])
}

// Dishes son los platos curados del Meal Closing Engine (dishes_catalog.csv).
// Se validan contra el catálogo de alimentos al arrancar.
//
//go:embed dishes_catalog.csv
var Dishes []byte
