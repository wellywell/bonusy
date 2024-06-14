package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	logger "github.com/sirupsen/logrus"
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
	ErrCouldNotParseBody = errors.New("could not parse body")
	ErrAuthDataEmpty     = errors.New("login or password cannot be empty")
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
		return "", "", ErrCouldNotParseBody
	}

	if data.Username == "" || data.Password == "" {
		return "", "", ErrAuthDataEmpty
	}

	return data.Username, data.Password, nil

}

func (h *HandlerSet) handleAuthErrors(err error, w http.ResponseWriter) {

	if errors.Is(err, ErrCouldNotParseBody) {
		http.Error(w, "Could not parse body",
			http.StatusBadRequest)
	} else if errors.Is(err, ErrAuthDataEmpty) {
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

func (h *HandlerSet) HandlePostWithdraw(w http.ResponseWriter, req *http.Request) {
	userID, err := h.handleAuthorizeUser(w, req)
	if err != nil {
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
		return
	}

	var data struct {
		Order string  `json:"order"`
		Sum   float64 `json:"sum"`
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		http.Error(w, "Could not parse body",
			http.StatusUnprocessableEntity)
		return
	}

	orderNum := string(data.Order)
	if !validate.ValidateOrderNumber(orderNum) {
		http.Error(w, "Invalid order number",
			http.StatusUnprocessableEntity)
		return
	}

	err = h.database.InsertWithdrawAndUpdateBalance(req.Context(), userID, data.Order, data.Sum)
	if err != nil && errors.Is(err, db.ErrNotEnoughBalance) {
		http.Error(w, "Not enough balance",
			http.StatusPaymentRequired)
		return
	}
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)

}

func (h *HandlerSet) HandlePostUserOrder(w http.ResponseWriter, req *http.Request) {

	userID, err := h.handleAuthorizeUser(w, req)
	if err != nil {
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
	w.WriteHeader(http.StatusAccepted)
}

func (h *HandlerSet) handleAuthorizeUser(w http.ResponseWriter, req *http.Request) (int, error) {
	username, ok := auth.GetAuthenticatedUser(req)
	if !ok {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
		return 0, fmt.Errorf("Authentication error")
	}

	userID, err := h.database.GetUserID(req.Context(), username)
	if err != nil {
		http.Error(w, "User not found",
			http.StatusUnauthorized)
		return 0, err
	}
	return userID, nil

}

func (h *HandlerSet) HandleGetUserOrders(w http.ResponseWriter, req *http.Request) {

	userID, err := h.handleAuthorizeUser(w, req)
	if err != nil {
		return
	}

	orders, err := h.database.GetUserOrders(req.Context(), userID)
	if err != nil {
		logger.Error(err)
		http.Error(w, "Error getting data", http.StatusInternalServerError)
		return
	}

	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	response, err := json.Marshal(orders)
	if err != nil {
		http.Error(w, "Could not serialize result",
			http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json")
	_, err = w.Write(response)
	if err != nil {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
	}
}

func (h *HandlerSet) HandleGetUserWithdrawals(w http.ResponseWriter, req *http.Request) {

	userID, err := h.handleAuthorizeUser(w, req)
	if err != nil {
		return
	}

	results, err := h.database.GetUserWithdrawals(req.Context(), userID)
	if err != nil {
		logger.Error(err)
		http.Error(w, "Error getting data", http.StatusInternalServerError)
		return
	}

	if len(results) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	response, err := json.Marshal(results)
	if err != nil {
		http.Error(w, "Could not serialize result",
			http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json")
	_, err = w.Write(response)
	if err != nil {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
	}
}

func (h *HandlerSet) HandleGetUserBalance(w http.ResponseWriter, req *http.Request) {

	userID, err := h.handleAuthorizeUser(w, req)
	if err != nil {
		return
	}

	balance, err := h.database.GetUserBalance(req.Context(), userID)
	if err != nil {
		logger.Error(err)
		http.Error(w, "Error getting data", http.StatusInternalServerError)
		return
	}

	response, err := json.Marshal(balance)
	if err != nil {
		http.Error(w, "Could not serialize result",
			http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json")
	_, err = w.Write(response)
	if err != nil {
		http.Error(w, "Something went wrong",
			http.StatusInternalServerError)
	}
}
