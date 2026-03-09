package admin

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (f *fakeEconomy) WithTransaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return fn(ctx, nil)
}

func (f *fakeEconomy) AddBalanceTx(ctx context.Context, tx pgx.Tx, userID int64, amount int64, txType, description string) error {
	return f.AddBalance(ctx, userID, amount, txType, description)
}
