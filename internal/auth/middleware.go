package auth

import (
	"context"
	"net/http"
)

type AuthenticateMiddleware struct {
	Secret []byte
}

type key string

const contextKey key = "username"

func (m AuthenticateMiddleware) Handle(next http.Handler) http.Handler {

	authenticate := func(w http.ResponseWriter, r *http.Request) {

		user, err := VerifyUser(r, m.Secret)
		if err != nil {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), contextKey, user)
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)

	}
	return http.HandlerFunc(authenticate)
}

func GetAuthenticatedUser(req *http.Request) (string, bool) {
	user, ok := req.Context().Value(contextKey).(string)
	return user, ok

}
