package main

import (
	"context"

	logger "github.com/sirupsen/logrus"
	"github.com/wellywell/bonusy/internal/accrual"
	"github.com/wellywell/bonusy/internal/config"
	"github.com/wellywell/bonusy/internal/db"
	"github.com/wellywell/bonusy/internal/handlers"
	"github.com/wellywell/bonusy/internal/order"
	"github.com/wellywell/bonusy/internal/router"
)

func main() {
	conf, err := config.NewConfig()
	if err != nil {
		panic(err)
	}

	logger.Info("Database on", conf.DatabaseDSN)
	database, err := db.NewDatabase(conf.DatabaseDSN)
	if err != nil {
		panic(err)
	}
	client := accrual.NewAccrualClient(conf.AccrualSystemAddress)

	ctx, cancel := context.WithCancel(context.Background())

	checkOrdersQueue := order.GenerateStatusTasks(ctx, database)
	updateOrdersQueue := order.CheckAccrualOrders(ctx, checkOrdersQueue, client)

	order.UpdateStatuses(ctx, updateOrdersQueue, database)

	handlerSet := handlers.NewHandlerSet(conf.Secret, conf.AuthCookieExpiresIn, database)

	r := router.NewRouter(conf, handlerSet)

	err = r.ListenAndServe()
	if err != nil {
		cancel()
		panic(err)
	}
}
