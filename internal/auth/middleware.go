package auth

import (
	"context"
	"errors"
	"net/http"
)

const sessionCookieName = "tinyprobe_session"

type contextKey int

const (
	adminContextKey contextKey = iota
	tokenContextKey
)

func RequireSession(service *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil || cookie.Value == "" {
				writeError(w, http.StatusUnauthorized, ErrUnauthenticated)
				return
			}

			admin, err := service.Authenticate(r.Context(), cookie.Value)
			if errors.Is(err, ErrUnauthenticated) {
				writeError(w, http.StatusUnauthorized, ErrUnauthenticated)
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, errors.New("internal server error"))
				return
			}

			ctx := context.WithValue(r.Context(), adminContextKey, admin)
			ctx = context.WithValue(ctx, tokenContextKey, cookie.Value)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func adminFromContext(ctx context.Context) Admin {
	admin, _ := ctx.Value(adminContextKey).(Admin)
	return admin
}

func tokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(tokenContextKey).(string)
	return token
}
