package domain

import (
	"fmt"
	"time"
)

// CreditBucket identifica de dónde sale (o a dónde va) un crédito.
type CreditBucket string

const (
	BucketDailyQuota   CreditBucket = "daily_quota"  // cuota gratuita diaria, no acumulable
	BucketSubscription CreditBucket = "subscription" // uso justo de Owli Pro
	BucketWelcome      CreditBucket = "welcome"      // regalo único al registrarse
	BucketPack         CreditBucket = "pack"         // comprados con IAP: no caducan nunca
)

// CreditKind es el tipo de movimiento del ledger.
type CreditKind string

const (
	KindGrant   CreditKind = "grant"   // alta de créditos (bienvenida, compra de pack)
	KindConsume CreditKind = "consume" // un análisis de IA
	KindRefund  CreditKind = "refund"  // compensación de un consumo (la IA falló o no detectó comida)
)

// CreditTransaction es una fila del ledger append-only `credit_transactions`.
// Nunca se modifica ni se borra: las correcciones son transacciones nuevas.
//
// Delta es positivo en grant/refund y negativo en consume. Reference vincula el
// movimiento con su origen (ID de evento de RevenueCat, ID del análisis…) y es
// la clave de idempotencia.
type CreditTransaction struct {
	ID        string
	UserID    string
	Kind      CreditKind
	Bucket    CreditBucket
	Delta     int
	Reference string
	CreatedAt time.Time
}

// CreditPolicy son los parámetros comerciales, cargados desde configuración.
type CreditPolicy struct {
	FreeDailyQuota  int // análisis IA gratis al día
	WelcomeCredits  int // créditos únicos al registrarse
	ProFairUseDaily int // tope técnico diario para Owli Pro
}

// Validate rechaza políticas incoherentes.
func (p CreditPolicy) Validate() error {
	if p.FreeDailyQuota < 0 || p.WelcomeCredits < 0 || p.ProFairUseDaily <= 0 {
		return fmt.Errorf("política de créditos inválida: %+v", p)
	}
	return nil
}

// CreditWallet es el estado del usuario en el momento de cobrar. El
// repositorio lo construye a partir del ledger dentro de una transacción con
// SELECT ... FOR UPDATE, de modo que dos análisis simultáneos no gasten el
// mismo crédito.
type CreditWallet struct {
	IsPro          bool
	UsedToday      int // consumos netos de daily_quota/subscription en el día local del usuario
	WelcomeBalance int
	PackBalance    int
}

// PickDebitBucket decide de qué bucket sale el próximo análisis.
//
// Orden de consumo: cuota diaria (o uso justo Pro) → bienvenida → packs.
// Los packs van al final porque el usuario pagó por ellos y no caducan.
// Un usuario Pro que agota el uso justo puede seguir con bienvenida o packs.
func (p CreditPolicy) PickDebitBucket(w CreditWallet) (CreditBucket, error) {
	if w.IsPro && w.UsedToday < p.ProFairUseDaily {
		return BucketSubscription, nil
	}
	if !w.IsPro && w.UsedToday < p.FreeDailyQuota {
		return BucketDailyQuota, nil
	}
	if w.WelcomeBalance > 0 {
		return BucketWelcome, nil
	}
	if w.PackBalance > 0 {
		return BucketPack, nil
	}
	if w.IsPro {
		return "", ErrFairUseExceeded
	}
	return "", ErrInsufficientCredits
}
