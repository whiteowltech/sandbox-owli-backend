// Package domain contiene los modelos y las reglas de negocio puras de Owli.
//
// Regla de capas: este paquete solo puede importar la biblioteca estándar.
// Nada de drivers SQL, net/http, Chi, Redis ni SDKs de IA.
package domain

import "errors"

// Errores centinela del dominio. El adapter HTTP es el único que los traduce a
// códigos de estado; aquí no existe el concepto de HTTP.
var (
	ErrInvalidBiometrics   = errors.New("biometría inválida")
	ErrUnderage            = errors.New("el usuario debe ser mayor de edad")
	ErrInvalidDiet         = errors.New("dieta desconocida")
	ErrInvalidRestriction  = errors.New("restricción alimentaria desconocida")
	ErrInvalidGoal         = errors.New("objetivo o ritmo inválido")
	ErrInvalidProfile      = errors.New("perfil inválido")
	ErrInvalidMacros       = errors.New("macros inválidos")
	ErrInsufficientCredits = errors.New("créditos insuficientes")
	ErrFairUseExceeded     = errors.New("límite diario de uso justo alcanzado")
)
