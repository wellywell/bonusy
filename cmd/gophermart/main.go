package main

import (
	"github.com/wellywell/bonusy/internal/config"
	"github.com/wellywell/bonusy/internal/db"
	"github.com/wellywell/bonusy/internal/handlers"
	"github.com/wellywell/bonusy/internal/router"
)

func main() {
	conf, err := config.NewConfig()
	if err != nil {
		panic(err)
	}

	database, err := db.NewDatabase(conf.DatabaseDSN)
	if err != nil {
		panic(err)
	}
	handlerSet := handlers.NewHandlerSet(conf.Secret, conf.AuthCookieExpiresIn, database)

	r := router.NewRouter(conf.RunAddress, handlerSet)

	err = r.ListenAndServe()
	if err != nil {
		panic(err)
	}

}
