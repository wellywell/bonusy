package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/wellywell/bonusy/internal/auth"
	"github.com/wellywell/bonusy/internal/db"
	"github.com/wellywell/bonusy/internal/types"
	"github.com/wellywell/bonusy/internal/validate"
)

type HandlerSet struct {
	secret               []byte
	cookieExpiresSeconds int
	database             *db.Database
}

var (
	couldNotParseBody = errors.New("could not parse body")
	authDataEmpty     = errors.New("login or password cannot be empty")
)

func NewHandlerSet(secret []byte, cookieExpiresSecs int, database *db.Database) *HandlerSet {
	return &HandlerSet{
		secret:               secret,
		cookieExpiresSeconds: cookieExpiresSecs,
		database:             database,
	}
}

func (h *HandlerSet) parseAuthData(body []byte) (username string, password string, err error) {

	var data struct {
		Username string `json:"login"`
		Password string `json:"password"`
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		return "", "", couldNotParseBody
	}

	if data.Username == "" || data.Password == "" {
		return "", "", authDataEmpty
	}

	return data.Username, data.Password, nil

}

func (h *HandlerSet) handleAuthErrors(err error, w http.ResponseWriter) {

	if errors.Is(err, couldNotParseBody) {
		http.Error(w, "Could not parse body",
			http.StatusBadRequest)
	} else if errors.Is(err, authDataEmpty) {
		http.Error(w, "Login and password cannot be empty",
			http.StatusBadRequest)
	} else {
		http.Error(w, "Unknown error", http.StatusInternalServerError)
	}
}

func (h *HandlerSet) HandleLogin(w http.ResponseWriter, req *http.Request) {

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
		return
	}

	username, password, err := h.parseAuthData(body)

	if err != nil {
		h.handleAuthErrors(err, w)
		return
	}

	passwordInDB, err := h.database.GetUserHashedPassword(req.Context(), username)
	if err != nil {
		var userNotFound *db.UserNotFoundError
		if errors.As(err, &userNotFound) {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !auth.CheckPasswordHash(password, passwordInDB) {
		http.Error(w, "Wrong password", http.StatusUnauthorized)
		return
	}

	err = auth.SetAuthCookie(username, w, h.secret, h.cookieExpiresSeconds)
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

func (h *HandlerSet) HandleRegisterUser(w http.ResponseWriter, req *http.Request) {

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
		return
	}

	username, password, err := h.parseAuthData(body)

	if err != nil {
		h.handleAuthErrors(err, w)
		return
	}

	hashed, err := auth.HashPassword(password)
	if err != nil {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
		return
	}

	err = h.database.CreateUser(req.Context(), username, hashed)
	if err != nil {
		var userExists *db.UserExistsError
		if errors.As(err, &userExists) {
			http.Error(w, "User exists", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = auth.SetAuthCookie(username, w, h.secret, h.cookieExpiresSeconds)
	if err != nil {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "text/plain")

	_, err = w.Write([]byte("success"))
	if err != nil {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
	}
}

func (h *HandlerSet) HandlePostUserOrder(w http.ResponseWriter, req *http.Request) {

	username, ok := auth.GetAuthenticatedUser(req)
	if !ok {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
		return
	}

	userID, err := h.database.GetUserID(req.Context(), username)
	if err != nil {
		http.Error(w, "User not found",
			http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
		return
	}

	orderNum := string(body)
	if !validate.ValidateOrderNumber(orderNum) {
		http.Error(w, "Invalid order number",
			http.StatusUnprocessableEntity)
		return
	}
	err = h.database.InsertUserOrder(req.Context(), orderNum, userID, types.NewStatus)
	if err != nil {
		var orderExistsSameUser *db.UserAlreadyUploadedOrder
		if errors.As(err, &orderExistsSameUser) {
			w.WriteHeader(http.StatusOK)
			return
		}
		var orderExistsWrongUser *db.OrderUploadedByWrongUser
		if errors.As(err, &orderExistsWrongUser) {
			http.Error(w, err.Error(),
				http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
