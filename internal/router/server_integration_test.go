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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/jackc/pgx/v5"
	"github.com/ory/dockertest"
	"github.com/ory/dockertest/docker"
	"github.com/stretchr/testify/assert"
	"github.com/wellywell/bonusy/internal/auth"
	"github.com/wellywell/bonusy/internal/config"
	"github.com/wellywell/bonusy/internal/db"
	"github.com/wellywell/bonusy/internal/handlers"
)

func TestMain(m *testing.M) {
	code, err := runMain(m)

	if err != nil {
		log.Fatal(err)
	}
	os.Exit(code)
}

const (
	testDBName       = "test"
	testUserName     = "test"
	testUserPassword = "test"
)

var (
	getDSN          func() string
	getSUConnection func() (*pgx.Conn, error)
)

func initGetDSN(hostAndPort string) {
	getDSN = func() string {
		return fmt.Sprintf(
			"postgres://%s:%s@%s/%s?sslmode=disable",
			testUserName,
			testUserPassword,
			hostAndPort,
			testDBName,
		)
	}
}

func initGetSUConnection(hostPort string) error {
	host, port, err := getHostPort(hostPort)
	if err != nil {
		return err
	}
	getSUConnection = func() (*pgx.Conn, error) {
		conn, err := pgx.Connect(context.Background(), fmt.Sprintf("postgres://postgres:postgres@%s:%d/postgres?sslmode=disable", host, port))
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
	return nil
}

func runMain(m *testing.M) (int, error) {
	pool, err := dockertest.NewPool("")

	if err != nil {
		return 1, err
	}

	pg, err := pool.RunWithOptions(
		&dockertest.RunOptions{
			Repository: "postgres",
			Tag:        "15.3",
			Name:       "test-bonusy-server",
			Env: []string{
				"POSTGRES_USER=postgres",
				"POSTGRES_PASSWORD=postgres",
			},
			ExposedPorts: []string{"5432"},
		},
		func(config *docker.HostConfig) {
			config.AutoRemove = true
			config.RestartPolicy = docker.RestartPolicy{Name: "no"}

		},
	)
	if err != nil {
		return 1, err
	}

	defer func() {
		if err := pool.Purge(pg); err != nil {
			log.Printf("Failed to purge docker")
		}
	}()

	hostPort := pg.GetHostPort("5432/tcp")
	initGetDSN(hostPort)
	if err := initGetSUConnection(hostPort); err != nil {
		return 1, err
	}

	pool.MaxWait = 10 * time.Second
	var conn *pgx.Conn
	if err := pool.Retry(func() error {
		conn, err = getSUConnection()
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return 1, err
	}

	defer func() {
		if err := conn.Close(context.Background()); err != nil {
			log.Printf("Error closing connection")
		}
	}()

	if err := createTestDB(conn); err != nil {
		return 1, err
	}

	database, err := db.NewDatabase(getDSN())
	if err != nil {
		panic(err)
	}
	handlerSet := handlers.NewHandlerSet([]byte("secret"), 1, database)

	config := config.ServerConfig{
		Secret:     []byte("secret"),
		RunAddress: "localhost:8080",
	}

	r := NewRouter(&config, handlerSet)

	go r.ListenAndServe()

	exitCode := m.Run()

	return exitCode, nil

}

func createTestDB(conn *pgx.Conn) error {
	_, err := conn.Exec(context.Background(),
		fmt.Sprintf(
			`CREATE USER %s PASSWORD '%s'`,
			testUserName,
			testUserPassword,
		),
	)
	if err != nil {
		return err
	}
	_, err = conn.Exec(
		context.Background(),
		fmt.Sprintf(`
		CREATE DATABASE %s
		OWNER '%s'
		ENCODING 'UTF8'`, testDBName, testUserName,
		),
	)
	if err != nil {
		return err
	}

	return nil
}

func getHostPort(hostPort string) (string, uint16, error) {
	parts := strings.Split(hostPort, ":")

	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, err
	}

	return parts[0], uint16(port), nil
}

func TestRegisterUser(t *testing.T) {

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
				conn, err := pgx.Connect(context.Background(), getDSN())
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
	testCases := []struct {
		method string
		body   string
		path   string
	}{
		{method: http.MethodPost, path: "http://localhost:8080/api/user/orders"},
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

	testCases := []struct {
		method       string
		body         string
		expectedCode int
		expectedBody string
	}{
		{method: http.MethodGet, body: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: ""},
		{method: http.MethodPut, body: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: ""},
		{method: http.MethodDelete, body: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: ""},
		{method: http.MethodPost, body: "", expectedCode: http.StatusBadRequest, expectedBody: "Invalid order number\n"},
		{method: http.MethodPost, body: "1", expectedCode: http.StatusBadRequest, expectedBody: "Invalid order number\n"},
		{method: http.MethodPost, body: "49927398716", expectedCode: http.StatusOK, expectedBody: ""},
	}

	cookie := getAuthCookie()

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {

			req := resty.New().R()
			req.Method = tc.method
			req.SetCookie(cookie)
			req.URL = "http://localhost:8080/api/user/orders"
			req.SetBody([]byte(tc.body))

			resp, err := req.Send()
			assert.NoError(t, err, "error making HTTP request")

			assert.Equal(t, tc.expectedCode, resp.StatusCode(), "Response code didn't match expected")
			assert.Equal(t, tc.expectedBody, string(resp.Body()))

		})
	}
}

func getAuthCookie() *http.Cookie {

	authData := []byte(`{"login" : "mylogin1", "password" : "mypassword1"}`)

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
