package domain

import (
	"errors"
	"testing"
)

func TestPickDebitBucket(t *testing.T) {
	policy := CreditPolicy{FreeDailyQuota: 1, WelcomeCredits: 15, ProFairUseDaily: 25}
	tests := []struct {
		name    string
		wallet  CreditWallet
		want    CreditBucket
		wantErr error
	}{
		{"gratis con cuota disponible", CreditWallet{UsedToday: 0, WelcomeBalance: 15}, BucketDailyQuota, nil},
		{"gratis, cuota agotada, usa bienvenida", CreditWallet{UsedToday: 1, WelcomeBalance: 2, PackBalance: 50}, BucketWelcome, nil},
		{"gratis, solo le quedan packs", CreditWallet{UsedToday: 1, PackBalance: 50}, BucketPack, nil},
		{"gratis sin nada", CreditWallet{UsedToday: 1}, "", ErrInsufficientCredits},
		{"pro dentro del uso justo", CreditWallet{IsPro: true, UsedToday: 24, PackBalance: 50}, BucketSubscription, nil},
		{"pro agota uso justo y usa packs", CreditWallet{IsPro: true, UsedToday: 25, PackBalance: 1}, BucketPack, nil},
		{"pro agota uso justo sin packs", CreditWallet{IsPro: true, UsedToday: 25}, "", ErrFairUseExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := policy.PickDebitBucket(tt.wallet)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, se esperaba %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("bucket = %q, se esperaba %q", got, tt.want)
			}
		})
	}
}

func TestCreditPolicyValidate(t *testing.T) {
	if err := (CreditPolicy{1, 15, 25}).Validate(); err != nil {
		t.Errorf("política válida rechazada: %v", err)
	}
	if err := (CreditPolicy{1, 15, 0}).Validate(); err == nil {
		t.Error("se esperaba error con ProFairUseDaily = 0")
	}
}
