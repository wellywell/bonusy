//go:build integration_tests
// +build integration_tests

/* В связи с санкциями, нужен VPN, чтобы докерхаб работал */

package router

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/wellywell/bonusy/internal/auth"
	"github.com/wellywell/bonusy/internal/config"
	"github.com/wellywell/bonusy/internal/db"
	"github.com/wellywell/bonusy/internal/handlers"
	"github.com/wellywell/bonusy/internal/testutils"
)

var DBDSN string

func TestMain(m *testing.M) {
	code, err := runMain(m)

	if err != nil {
		log.Fatal(err)
	}
	os.Exit(code)
}

func runMain(m *testing.M) (int, error) {

	databaseDSN, cleanUp, err := testutils.RunTestDatabase()
	defer cleanUp()

	if err != nil {
		return 1, err
	}

	DBDSN = databaseDSN

	database, err := db.NewDatabase(DBDSN)
	if err != nil {
		return 1, err
	}
	handlerSet := handlers.NewHandlerSet([]byte("secret"), 1, database)

	config := config.ServerConfig{
		Secret:      []byte("secret"),
		RunAddress:  "localhost:8080",
		DatabaseDSN: DBDSN,
	}

	r := NewRouter(&config, handlerSet)

	go r.ListenAndServe()

	exitCode := m.Run()

	return exitCode, nil

}

func TestRegisterUser(t *testing.T) {

	cleanUp(t)

	goodBody := `{"login" : "mylogin", "password" : "mypassword"}`
	emptyData1 := `{"login" : "", "password" : "mypassword"}`
	emptyData2 := `{"login" : "a", "password" : ""}`
	wrongBody := "smth"

	testCases := []struct {
		method       string
		body         string
		expectedCode int
		expectedBody string
	}{
		{method: http.MethodGet, body: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: ""},
		{method: http.MethodPut, body: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: ""},
		{method: http.MethodDelete, body: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: ""},
		{method: http.MethodPost, body: wrongBody, expectedCode: http.StatusBadRequest, expectedBody: "Could not parse body\n"},
		{method: http.MethodPost, body: emptyData1, expectedCode: http.StatusBadRequest, expectedBody: "Login and password cannot be empty\n"},
		{method: http.MethodPost, body: emptyData2, expectedCode: http.StatusBadRequest, expectedBody: "Login and password cannot be empty\n"},
		{method: http.MethodPost, body: goodBody, expectedCode: http.StatusOK, expectedBody: "success"},
		{method: http.MethodPost, body: goodBody, expectedCode: http.StatusConflict, expectedBody: "User exists\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {

			req := resty.New().R()
			req.Method = tc.method
			req.URL = "http://localhost:8080/api/user/register"
			req.SetBody([]byte(tc.body))

			resp, err := req.Send()
			assert.NoError(t, err, "error making HTTP request")

			assert.Equal(t, tc.expectedCode, resp.StatusCode(), "Response code didn't match expected")
			assert.Equal(t, tc.expectedBody, string(resp.Body()))

			if tc.expectedCode == http.StatusOK {

				// check user in DB
				conn, err := pgx.Connect(context.Background(), DBDSN)
				if err != nil {
					panic(err)
				}
				row := conn.QueryRow(context.Background(), "SELECT username, password FROM auth_user WHERE username = $1", "mylogin")
				var login string
				var password string
				if err := row.Scan(&login, &password); err != nil {
					panic(err)
				}
				assert.Equal(t, login, "mylogin")
				assert.NotEqual(t, password, "mypassword") // test passsword not in plaintext

				// check cookie set
				assert.NotEmpty(t, resp.Cookies())

				// check user in cookie correct
				cookie := resp.Cookies()[0]
				user, err := auth.GetUser(cookie.Value, []byte("secret"))
				if err != nil {
					panic(err)
				}
				assert.Equal(t, user, "mylogin")

			}
		})
	}
}

func TestLoginUserNotExists(t *testing.T) {

	cleanUp(t)

	goodBody := `{"login" : "mylogin1", "password" : "mypassword1"}`
	emptyData1 := `{"login" : "", "password" : "mypassword1"}`
	emptyData2 := `{"login" : "a", "password" : ""}`
	wrongBody := "smth"

	testCases := []struct {
		method       string
		body         string
		expectedCode int
		expectedBody string
	}{
		{method: http.MethodGet, body: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: ""},
		{method: http.MethodPut, body: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: ""},
		{method: http.MethodDelete, body: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: ""},
		{method: http.MethodPost, body: wrongBody, expectedCode: http.StatusBadRequest, expectedBody: "Could not parse body\n"},
		{method: http.MethodPost, body: emptyData1, expectedCode: http.StatusBadRequest, expectedBody: "Login and password cannot be empty\n"},
		{method: http.MethodPost, body: emptyData2, expectedCode: http.StatusBadRequest, expectedBody: "Login and password cannot be empty\n"},
		{method: http.MethodPost, body: goodBody, expectedCode: http.StatusUnauthorized, expectedBody: "User not found\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {

			req := resty.New().R()
			req.Method = tc.method
			req.URL = "http://localhost:8080/api/user/login"
			req.SetBody([]byte(tc.body))

			resp, err := req.Send()
			assert.NoError(t, err, "error making HTTP request")

			assert.Equal(t, tc.expectedCode, resp.StatusCode(), "Response code didn't match expected")
			assert.Equal(t, tc.expectedBody, string(resp.Body()))
		})
	}
}

func TestRegisterAndLogin(t *testing.T) {

	cleanUp(t)

	goodBody := `{"login" : "mylogin1", "password" : "mypassword1"}`
	emptyData1 := `{"login" : "", "password" : "mypassword1"}`
	emptyData2 := `{"login" : "a", "password" : ""}`
	wrongPassword := `{"login" : "mylogin1", "password" : "wrong"}`
	wrongBody := "smth"

	testCases := []struct {
		method       string
		body         string
		expectedCode int
		expectedBody string
	}{
		{method: http.MethodPost, body: wrongBody, expectedCode: http.StatusBadRequest, expectedBody: "Could not parse body\n"},
		{method: http.MethodPost, body: emptyData1, expectedCode: http.StatusBadRequest, expectedBody: "Login and password cannot be empty\n"},
		{method: http.MethodPost, body: emptyData2, expectedCode: http.StatusBadRequest, expectedBody: "Login and password cannot be empty\n"},
		{method: http.MethodPost, body: wrongPassword, expectedCode: http.StatusUnauthorized, expectedBody: "Wrong password\n"},
		{method: http.MethodPost, body: goodBody, expectedCode: http.StatusOK, expectedBody: "success"},
	}

	// register user first
	req := resty.New().R()
	req.Method = http.MethodPost
	req.URL = "http://localhost:8080/api/user/register"
	req.SetBody([]byte(goodBody))

	_, err := req.Send()
	assert.NoError(t, err, "error making HTTP request")

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {

			req := resty.New().R()
			req.Method = tc.method
			req.URL = "http://localhost:8080/api/user/login"
			req.SetBody([]byte(tc.body))

			resp, err := req.Send()
			assert.NoError(t, err, "error making HTTP request")

			assert.Equal(t, tc.expectedCode, resp.StatusCode(), "Response code didn't match expected")
			assert.Equal(t, tc.expectedBody, string(resp.Body()))

			if tc.expectedCode == http.StatusOK {
				// check cookie set
				assert.NotEmpty(t, resp.Cookies())
				// check user in cookie correct
				cookie := resp.Cookies()[0]
				user, err := auth.GetUser(cookie.Value, []byte("secret"))
				if err != nil {
					panic(err)
				}
				assert.Equal(t, user, "mylogin1")
			}
		})
	}
}

func TestNotAuthenticated(t *testing.T) {
	cleanUp(t)
	testCases := []struct {
		method string
		body   string
		path   string
	}{
		{method: http.MethodPost, path: "http://localhost:8080/api/user/orders"},
		{method: http.MethodGet, path: "http://localhost:8080/api/user/orders"},
		{method: http.MethodGet, path: "http://localhost:8080/api/user/balance"},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {

			req := resty.New().R()
			req.Method = tc.method
			req.URL = tc.path

			resp, _ := req.Send()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
		})
	}

}

func TestPostUser(t *testing.T) {

	cleanUp(t)

	cookie := getAuthCookie(t, "user1", "passw")
	otherUserCookie := getAuthCookie(t, "user2", "passw")

	testCases := []struct {
		method       string
		body         string
		expectedCode int
		expectedBody string
		cookie       *http.Cookie
	}{
		{method: http.MethodPut, body: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: "", cookie: cookie},
		{method: http.MethodDelete, body: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: "", cookie: cookie},
		{method: http.MethodPost, body: "", expectedCode: http.StatusUnprocessableEntity, expectedBody: "Invalid order number\n", cookie: cookie},
		{method: http.MethodPost, body: "1", expectedCode: http.StatusUnprocessableEntity, expectedBody: "Invalid order number\n", cookie: cookie},
		{method: http.MethodPost, body: "49927398716", expectedCode: http.StatusAccepted, expectedBody: "", cookie: cookie},
		{method: http.MethodPost, body: "49927398716", expectedCode: http.StatusOK, expectedBody: "", cookie: cookie},
		{method: http.MethodPost, body: "49927398716", expectedCode: http.StatusConflict, expectedBody: "Other user already uploaded order 49927398716\n", cookie: otherUserCookie},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {

			req := resty.New().R()
			req.Method = tc.method
			req.SetCookie(tc.cookie)
			req.URL = "http://localhost:8080/api/user/orders"
			req.SetBody([]byte(tc.body))

			resp, err := req.Send()
			assert.NoError(t, err, "error making HTTP request")

			assert.Equal(t, tc.expectedCode, resp.StatusCode(), "Response code didn't match expected")
			assert.Equal(t, tc.expectedBody, string(resp.Body()))

		})
	}
}

func TestGetUserOrders(t *testing.T) {

	cleanUp(t)

	cookie := getAuthCookie(t, "user1", "passw")
	otherUserCookie := getAuthCookie(t, "user2", "passw")

	ti := "2024-06-12T15:13:29.681099+03:00"

	uploadedAt, err := time.Parse(time.RFC3339, ti)
	if err != nil {
		log.Fatal(err)
	}

	testCases := []struct {
		createOrders []string
		expectedCode int
		expectedBody string
		cookie       *http.Cookie
	}{

		{createOrders: []string{}, expectedCode: http.StatusNoContent, expectedBody: "", cookie: cookie},
		{createOrders: []string{"79927398713"}, expectedCode: http.StatusNoContent, expectedBody: "", cookie: otherUserCookie},
		{createOrders: []string{"49927398716"}, expectedCode: http.StatusOK, expectedBody: fmt.Sprintf("[{\"number\":\"49927398716\",\"status\":\"NEW\",\"accrual\":null,\"uploaded_at\": \"%s\"}]", ti), cookie: cookie},
		{createOrders: []string{"49927398716", "0"}, expectedCode: http.StatusOK, expectedBody: fmt.Sprintf("[{\"number\":\"49927398716\",\"status\":\"NEW\",\"accrual\":null,\"uploaded_at\": \"%s\"}, {\"number\":\"0\",\"status\":\"NEW\",\"accrual\":null,\"uploaded_at\": \"%s\"}]", ti, ti), cookie: cookie},
	}

	for _, tc := range testCases {
		t.Run(tc.expectedBody, func(t *testing.T) {

			// create orders
			for _, num := range tc.createOrders {
				req := resty.New().R()
				req.Method = http.MethodPost
				req.SetCookie(tc.cookie)
				req.URL = "http://localhost:8080/api/user/orders"
				req.SetBody([]byte(num))
				req.Send()
			}
			setTestDateTime(uploadedAt)

			req := resty.New().R()
			req.Method = http.MethodGet
			req.SetCookie(cookie)
			req.URL = "http://localhost:8080/api/user/orders"
			resp, err := req.Send()
			assert.NoError(t, err)

			assert.Equal(t, tc.expectedCode, resp.StatusCode(), "Response code didn't match expected")
			if tc.expectedCode == http.StatusOK {
				assert.JSONEq(t, tc.expectedBody, string(resp.Body()))
			} else {
				assert.Equal(t, tc.expectedBody, string(resp.Body()))
			}

		})
	}
}

func TestGetUserBalance(t *testing.T) {

	cleanUp(t)

	cookie := getAuthCookie(t, "user1", "passw")

	ctx := context.Background()
	database, err := db.NewDatabase(DBDSN)
	if err != nil {
		log.Fatal(err)
	}

	testCases := []struct {
		addUserBalance int
		expectedBody   string
	}{
		{0, `{"current": 0, "withdrawn": 0}`},
		{500, `{"current": 500, "withdrawn": 0}`},
		{1, `{"current": 501, "withdrawn": 0}`},
	}

	for _, tc := range testCases {
		t.Run("expectedBody", func(t *testing.T) {

			if tc.addUserBalance > 0 {
				userID, _ := database.GetUserID(ctx, "user1")
				database.InsertUserOrder(ctx, "0", userID, "NEW")
				err = database.UpdateOrder(ctx, 1, "PROCESSED", tc.addUserBalance)
				assert.NoError(t, err)
			}

			req := resty.New().R()
			req.Method = http.MethodGet
			req.SetCookie(cookie)
			req.URL = "http://localhost:8080/api/user/balance"
			resp, err := req.Send()
			assert.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode(), "Response code didn't match expected")
			assert.JSONEq(t, tc.expectedBody, string(resp.Body()))
		})
	}
}

func cleanUp(t *testing.T) {
	t.Cleanup(func() {
		conn, err := pgx.Connect(context.Background(), DBDSN)
		if err != nil {
			panic(err)
		}
		conn.Exec(context.Background(), "TRUNCATE TABLE auth_user RESTART IDENTITY CASCADE")
		conn.Exec(context.Background(), "TRUNCATE TABLE user_order RESTART IDENTITY CASCADE")
		conn.Exec(context.Background(), "TRUNCATE TABLE balance RESTART IDENTITY CASCADE")
	})

}

func setTestDateTime(d time.Time) {
	conn, err := pgx.Connect(context.Background(), DBDSN)
	if err != nil {
		log.Fatal(err)
	}
	_, err = conn.Exec(context.Background(), "UPDATE user_order SET uploaded_at = $1", d)
	if err != nil {
		log.Fatal(err)
	}
}

func getAuthCookie(t *testing.T, login string, password string) *http.Cookie {

	authData := []byte(fmt.Sprintf(`{"login" : "%s", "password" : "%s"}`, login, password))

	// В зависимости от порядка тестов, пользователь может быть уже зарегистрирован
	// Или же нужно создать нового
	req := resty.New().R()
	req.Method = http.MethodPost
	req.URL = "http://localhost:8080/api/user/register"
	req.SetBody(authData)
	req.Send()

	req = resty.New().R()
	req.Method = http.MethodPost
	req.URL = "http://localhost:8080/api/user/login"
	req.SetBody(authData)

	resp, _ := req.Send()
	cookie := resp.Cookies()[0]
	return cookie

}
