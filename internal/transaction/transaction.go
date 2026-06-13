package transaction

import (
	"context"
	"time"
)

// TransactionType là các loại giao dịch kinh tế.
type TransactionType string

const (
	TypeDeposit     TransactionType = "deposit"
	TypeWithdraw    TransactionType = "withdraw"
	TypeTransferIn  TransactionType = "transfer_in"
	TypeTransferOut TransactionType = "transfer_out"
	TypeBetWin      TransactionType = "bet_win"
	TypeBetLose     TransactionType = "bet_lose"
	TypeAdminGive   TransactionType = "admin_give"
	TypeAdminTake   TransactionType = "admin_take"
)

// AuditEntry là một bản ghi sử dụng cho audit log và replay.
type AuditEntry struct {
	Timestamp time.Time       `bson:"timestamp"`
	UserID    string          `bson:"user_id"`
	Type      TransactionType `bson:"type"`
	Amount    int64           `bson:"amount"`
	Before    int64           `bson:"before"`
	After     int64           `bson:"after"`
	RefID     string          `bson:"ref_id,omitempty"`
	Meta      string          `bson:"meta,omitempty"`
}

// Repository định nghĩa interface audit/transaction.
type Repository interface {
	Log(ctx context.Context, entry *AuditEntry) error
}
