package order

import (
	"context"
	"errors"
	"log"
	"time"

	logger "github.com/sirupsen/logrus"
	"github.com/wellywell/bonusy/internal/accrual"
	"github.com/wellywell/bonusy/internal/types"
)

type OrderUpdate struct {
	order  types.OrderRecord
	status accrual.OrderStatus
}

type AccrualClient interface {
	GetOrderStatus(orderNum string) (*accrual.OrderStatus, error)
}

type Database interface {
	GetUnprocessedOrders(ctx context.Context, startID int, limit int) ([]types.OrderRecord, error)
	UpdateUnprocessedOrder(ctx context.Context, orderID int, newStatus types.Status, accrual int) error
}

func GenerateStatusTasks(ctx context.Context, database Database) chan types.OrderRecord {

	tasks := make(chan types.OrderRecord)

	go func(ctx context.Context) {
		defer close(tasks)

		startID := 0
		limit := 100

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			records, err := database.GetUnprocessedOrders(ctx, startID, limit)
			if err != nil {
				logger.Error("Database returned error, stopping")
				log.Fatal(err)
			}
			if len(records) == 0 {
				logger.Info("All orders in DB were checked")
				time.Sleep(30 * time.Second)
				startID = 0
			}
			for _, task := range records {
				logger.Infof("Checking order %v", task)
				if task.OrderID > startID {
					startID = task.OrderID
				}
				tasks <- task
			}
		}
	}(ctx)

	return tasks
}

func CheckAccrualOrders(ctx context.Context, tasks <-chan types.OrderRecord, client AccrualClient) chan OrderUpdate {

	updates := make(chan OrderUpdate)

	go func(ctx context.Context) {
		defer close(updates)
		for {
			select {
			case <-ctx.Done():
				logger.Info("Context cancel, stopping processor")
				return
			case task, ok := <-tasks:
				if !ok {
					return
				}
				result, err := retryThrottle(task.OrderNum, client)
				if err != nil {
					if errors.Is(err, accrual.ErrOrderNotExists) {
						logger.Infof("Order %s not found", task.OrderNum)
						continue
					}
					if errors.Is(err, accrual.ErrUnknown) {
						logger.Errorf("Unknwon accrual error %s", err.Error())
						continue
					}
					logger.Error(err)
					continue
				}
				if result.Status == task.Status {
					continue
				}
				logger.Infof("Got order update %v", result)
				update := OrderUpdate{
					order:  task,
					status: *result,
				}
				updates <- update
			}
		}
	}(ctx)

	return updates
}

func retryThrottle(order string, client AccrualClient) (*accrual.OrderStatus, error) {

	for {
		result, err := client.GetOrderStatus(order)

		if err != nil {
			var errThrottle *accrual.ErrThrottle
			if !errors.As(err, &errThrottle) {
				return nil, err
			}
			sleep := errThrottle.RetryAfter
			logger.Warningf("Accrual too many requests, will retry in %d seconds", sleep)
			time.Sleep(time.Duration(sleep) * time.Second)

		} else {
			return result, err
		}
	}
}

func UpdateStatuses(ctx context.Context, tasks <-chan OrderUpdate, database Database) {
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case task, ok := <-tasks:
				if !ok {
					return
				}
				err := database.UpdateUnprocessedOrder(ctx, task.order.OrderID, task.status.Status, task.status.Accrual)
				if err != nil {
					logger.Error(err.Error())
				} else {
					logger.Infof("Updated order %s in database, new status: %s", task.order.OrderNum, task.status.Status)
				}
			}
		}
	}(ctx)
}
