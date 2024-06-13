package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/wellywell/bonusy/internal/auth"
	"github.com/wellywell/bonusy/internal/config"
	"github.com/wellywell/bonusy/internal/handlers"
)

const (
	compressLevel = 5
)

type Middleware interface {
	Handle(h http.Handler) http.Handler
}

type Router struct {
	address string
	router  *chi.Mux
}

func NewRouter(conf *config.ServerConfig, h *handlers.HandlerSet, middlewares ...Middleware) *Router {

	r := chi.NewRouter()

	for _, m := range middlewares {
		r.Use(m.Handle)
	}
	//r.Use(middleware.Logger)
	r.Use(middleware.Compress(compressLevel)) // TODO test

	r.Post("/api/user/register", h.HandleRegisterUser)
	r.Post("/api/user/login", h.HandleLogin)

	authMiddleware := &auth.AuthenticateMiddleware{Secret: conf.Secret}

	r.Group(func(r chi.Router) {

		r.Use(authMiddleware.Handle)
		r.Post("/api/user/orders", h.HandlePostUserOrder)
		r.Get("/api/user/orders", h.HandleGetUserOrders)
		r.Get("/api/user/balance", h.HandleGetUserBalance)
	})

	return &Router{router: r, address: conf.RunAddress}
}

func (r *Router) ListenAndServe() error {
	err := http.ListenAndServe(r.address, r.router)
	return err
}
