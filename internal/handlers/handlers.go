package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/wellywell/bonusy/internal/auth"
	"github.com/wellywell/bonusy/internal/db"
)

type HandlerSet struct {
	secret               []byte
	cookieExpiresSeconds int
	database             *db.Database
}

func NewHandlerSet(secret []byte, cookieExpiresSecs int, database *db.Database) *HandlerSet {
	return &HandlerSet{
		secret:               secret,
		cookieExpiresSeconds: cookieExpiresSecs,
		database:             database,
	}
}

func (h *HandlerSet) HandleRegisterUser(w http.ResponseWriter, req *http.Request) {

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
		return
	}

	var data struct {
		Username string `json:"login"`
		Password string `json:"password"`
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		http.Error(w, "Could not parse body",
			http.StatusBadRequest)
		return
	}

	if data.Username == "" || data.Password == "" {
		http.Error(w, "Login and password cannot be empty",
			http.StatusBadRequest)
		return
	}

	hashed, err := auth.HashPassword(data.Password)
	if err != nil {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
		return
	}

	err = h.database.CreateUser(req.Context(), data.Username, hashed)
	if err != nil {
		var userExists *db.UserExistsError
		if errors.As(err, &userExists) {
			http.Error(w, "User exists", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = auth.SetAuthCookie(data.Username, w, h.secret, h.cookieExpiresSeconds)
	if err != nil {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
	}

	w.Header().Set("content-type", "text/plain")

	_, err = w.Write([]byte("success"))
	if err != nil {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
	}
}
