package config

import (
	"flag"

	"github.com/caarlos0/env/v6"
)

/*
адрес и порт запуска сервиса: переменная окружения ОС RUN_ADDRESS или флаг -a;
адрес подключения к базе данных: переменная окружения ОС DATABASE_URI или флаг -d;
адрес системы расчёта начислений: переменная окружения ОС ACCRUAL_SYSTEM_ADDRESS или флаг -r.
*/

type ServerConfig struct {
	RunAddress           string `env:"RUN_ADDRESS"`
	AccrualSystemAddress string `env:"ACCRUAL_SYSTEM_ADDRESS"`
	DatabaseDSN          string `env:"DATABASE_URI"`
}

func NewConfig() (*ServerConfig, error) {
	var params ServerConfig
	err := env.Parse(&params)
	if err != nil {
		return nil, err
	}

	var commandLineParams ServerConfig

	flag.StringVar(&commandLineParams.RunAddress, "a", "localhost:8080", "Base address to listen on")
	flag.StringVar(&commandLineParams.AccrualSystemAddress, "r", "http://localhost:8080", "Accural system address")
	flag.StringVar(&commandLineParams.DatabaseDSN, "d", "postgres://postgres@localhost:5432/bonusy?sslmode=disable", "Database DSN")
	flag.Parse()

	if params.RunAddress == "" {
		params.RunAddress = commandLineParams.RunAddress
	}
	if params.AccrualSystemAddress == "" {
		params.AccrualSystemAddress = commandLineParams.AccrualSystemAddress
	}
	if params.DatabaseDSN == "" {
		params.DatabaseDSN = commandLineParams.DatabaseDSN
	}

	return &params, nil
}
