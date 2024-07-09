package testutils

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/ory/dockertest"
	"github.com/ory/dockertest/docker"
	logger "github.com/sirupsen/logrus"
)

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
	logger.Info("Setting up sudo connection to db")
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

func createTestDB(conn *pgx.Conn) error {
	logger.Info("Creating user and database")
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
	logger.Info("Getting host and port")
	parts := strings.Split(hostPort, ":")

	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, err
	}

	return parts[0], uint16(port), nil
}

func RunTestDatabase() (DSN string, cleanUp func(), err error) {

	var cleanUpfuncs []func()

	clear := func() {
		for _, f := range cleanUpfuncs {
			f()
		}
	}

	logger.Info("Starting docker pool")
	pool, err := dockertest.NewPool("")

	if err != nil {
		return "", clear, err
	}

	logger.Info("Creating DB container")

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
		return "", clear, err
	}
	cleanUpfuncs = append(cleanUpfuncs, func() {
		logger.Info("Closing docker pool")
		if err := pool.Purge(pg); err != nil {
			log.Printf("Failed to purge docker")
		}

	})

	hostPort := pg.GetHostPort("5432/tcp")

	initGetDSN(hostPort)
	if err := initGetSUConnection(hostPort); err != nil {
		return "", clear, err
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
		return "", clear, err
	}

	cleanUpfuncs = append(cleanUpfuncs, func() {
		logger.Info("Closing DB connection")
		if err := conn.Close(context.Background()); err != nil {
			log.Printf("Error closing connection")
		}
	})

	if err := createTestDB(conn); err != nil {
		return "", clear, err
	}

	dsn := getDSN()
	logger.Info("Test DSN: ", dsn)

	return dsn, clear, nil

}
