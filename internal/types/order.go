package types

import "time"

type Status string

const (
	NewStatus        Status = "NEW"
	ProcessingStatus Status = "PROCESSING"
	InvalidStatus    Status = "INVALID"
	ProcessedStatus  Status = "PROCESSED"
	RegisteredStatus Status = "REGISTERED"
)

type OrderRecord struct {
	OrderNum string `db:"order_number"`
	Status   Status `db:"status"`
	OrderID  int    `db:"id"`
}

type OrderInfo struct {
	Number     string    `db:"order_number" json:"number"`
	Status     Status    `db:"status" json:"status"`
	Accrual    *int      `db:"accrual" json:"accrual"`
	UploadedAt time.Time `db:"uploaded_at" json:"uploaded_at"`
}
