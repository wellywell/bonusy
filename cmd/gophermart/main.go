package main

import (
	"github.com/wellywell/bonusy/internal/config"
	"github.com/wellywell/bonusy/internal/db"
)

func main() {
	conf, err := config.NewConfig()
	if err != nil {
		panic(err)
	}
	db.Configure(conf.DatabaseDSN)
}
