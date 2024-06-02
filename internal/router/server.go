package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/wellywell/bonusy/internal/handlers"
)

type Middleware interface {
	Handle(h http.Handler) http.Handler
}

type Router struct {
	address string
	router  *chi.Mux
}

func NewRouter(address string, handl *handlers.HandlerSet, middlewares ...Middleware) *Router {

	r := chi.NewRouter()

	for _, m := range middlewares {
		r.Use(m.Handle)
	}
	r.Post("/api/user/register", handl.HandleRegisterUser)

	return &Router{router: r, address: address}
}

func (r *Router) ListenAndServe() error {
	err := http.ListenAndServe(r.address, r.router)
	return err
}
