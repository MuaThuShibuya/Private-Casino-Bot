package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	TxDeposit     = "deposit"
	TxWithdraw    = "withdraw"
	TxTransferIn  = "transfer_in"
	TxTransferOut = "transfer_out"
	TxBetWin      = "bet_win"
	TxBetLose     = "bet_lose"
	TxAdminGive   = "admin_give"
	TxAdminTake   = "admin_take"
	TxRefund      = "refund"
)

type TransactionLog struct {
	ID            primitive.ObjectID `bson:"_id,omitempty"`
	UserID        string             `bson:"user_id"`
	Type          string             `bson:"type"`
	Amount        int64              `bson:"amount"`
	BalanceBefore int64              `bson:"balance_before"`
	BalanceAfter  int64              `bson:"balance_after"`
	Description   string             `bson:"description"`
	RefID         string             `bson:"ref_id,omitempty"`
	CreatedAt     time.Time          `bson:"created_at"`
}
