package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/whiteowltech/owli/backend/internal/domain"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

// apiError es la forma de todos los errores: {"error": {"code", "message"}}.
// code es estable (la app decide con él); message es para humanos.
type apiError struct {
	Status  int
	Code    string
	Message string
}

// classify traduce errores de dominio y de puertos a HTTP. Es el único lugar
// donde existe esta correspondencia.
func classify(err error) apiError {
	switch {
	case errors.Is(err, domain.ErrUnderage):
		return apiError{http.StatusUnprocessableEntity, "underage", "Owli es para mayores de 18 años."}
	case errors.Is(err, domain.ErrInvalidBiometrics),
		errors.Is(err, domain.ErrInvalidGoal),
		errors.Is(err, domain.ErrInvalidDiet),
		errors.Is(err, domain.ErrInvalidRestriction),
		errors.Is(err, domain.ErrInvalidProfile):
		return apiError{http.StatusUnprocessableEntity, "invalid_profile", err.Error()}
	case errors.Is(err, domain.ErrInvalidMacros):
		return apiError{http.StatusUnprocessableEntity, "invalid_entry", err.Error()}
	case errors.Is(err, domain.ErrInsufficientCredits):
		return apiError{http.StatusPaymentRequired, "no_credits", "No te quedan análisis con IA por hoy. Anotar a mano sigue siendo gratis."}
	case errors.Is(err, domain.ErrFairUseExceeded):
		return apiError{http.StatusTooManyRequests, "fair_use_exceeded", "Llegaste al tope de uso justo de hoy. Mañana se renueva."}
	case errors.Is(err, ports.ErrAIUnavailable):
		return apiError{http.StatusServiceUnavailable, "ai_unavailable", "No pudimos hacer el análisis. No se usó ningún crédito: inténtalo de nuevo."}
	case errors.Is(err, ports.ErrInvalidTranscript):
		return apiError{http.StatusUnprocessableEntity, "invalid_transcript", "No entendimos lo que dijiste: inténtalo de nuevo (máximo 500 caracteres)."}
	case errors.Is(err, ports.ErrProfileRequired):
		return apiError{http.StatusConflict, "profile_required", "Primero completa tu perfil."}
	case errors.Is(err, ports.ErrInvalidCredentials):
		return apiError{http.StatusUnauthorized, "invalid_credentials", "No pudimos verificar tu identidad."}
	case errors.Is(err, ports.ErrSessionNotFound),
		errors.Is(err, ports.ErrSessionExpired),
		errors.Is(err, ports.ErrSessionReused):
		return apiError{http.StatusUnauthorized, "invalid_session", "Tu sesión terminó. Vuelve a entrar."}
	case errors.Is(err, ports.ErrIdentityInUse):
		return apiError{http.StatusConflict, "identity_in_use", "Esa cuenta ya está vinculada a otro perfil de Owli."}
	case errors.Is(err, ports.ErrProviderNotConfigured):
		return apiError{http.StatusBadRequest, "provider_not_available", "Ese método de acceso no está disponible."}
	case errors.Is(err, ports.ErrNotFound):
		return apiError{http.StatusNotFound, "not_found", "No encontrado."}
	case errors.Is(err, ports.ErrDependencyUnavailable):
		return apiError{http.StatusServiceUnavailable, "unavailable", "El servicio no está disponible en este momento. Prueba de nuevo."}
	}
	return apiError{http.StatusInternalServerError, "internal", "Algo falló de nuestro lado."}
}

// writeError responde el error. Los 5xx se registran con el detalle interno,
// que nunca se envía al cliente.
func writeError(w http.ResponseWriter, log *slog.Logger, e apiError, cause ...error) {
	if e.Status >= 500 && log != nil && len(cause) > 0 {
		log.Error("error interno", "status", e.Status, "code", e.Code, "err", cause[0])
	}
	writeJSON(w, e.Status, map[string]any{"error": map[string]string{"code": e.Code, "message": e.Message}})
}

func fail(w http.ResponseWriter, log *slog.Logger, err error) {
	writeError(w, log, classify(err), err)
}
