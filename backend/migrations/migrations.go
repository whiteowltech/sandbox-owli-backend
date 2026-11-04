// Package migrations contiene las migraciones SQL, embebidas en el binario para
// que el servidor las aplique al arrancar (también en Railway, sin herramientas extra).
package migrations

import "embed"

// FS contiene los archivos NNNN_nombre.sql en orden lexicográfico de aplicación.
//
//go:embed *.sql
var FS embed.FS
