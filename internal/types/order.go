package types

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
