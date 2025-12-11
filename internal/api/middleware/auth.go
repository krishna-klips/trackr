package middleware

import (
	"context"
	"net/http"
	"strings"

	apiContext "trackr/internal/api/context"
	"trackr/internal/pkg/errors"
	"trackr/internal/platform/auth"
)

type AuthMiddleware struct {
	tokenSvc *auth.TokenService
}

func NewAuthMiddleware(tokenSvc *auth.TokenService) *AuthMiddleware {
	return &AuthMiddleware{tokenSvc: tokenSvc}
}

func (m *AuthMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			errors.WriteError(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized, "Missing authorization header", nil)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			errors.WriteError(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized, "Invalid authorization header format", nil)
			return
		}

		claims, err := m.tokenSvc.ValidateToken(parts[1])
		if err != nil {
			errors.WriteError(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized, "Invalid or expired token", nil)
			return
		}

		ctx := context.WithValue(r.Context(), apiContext.Claims, claims)
		next(w, r.WithContext(ctx))
	}
}
