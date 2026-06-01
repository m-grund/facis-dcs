package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type SigningTaskData struct {
	ID        string    `db:"id"`
	DID       string    `db:"did"`
	State     string    `db:"state"`
	Signer    string    `db:"signer"`
	CreatedBy string    `db:"created_by"`
	CreatedAt time.Time `db:"created_at"`
}

type SigningTaskRepo interface {
	Create(ctx context.Context, tx *sqlx.Tx, data SigningTaskData) (*time.Time, error)
	ReopenTasks(ctx context.Context, tx *sqlx.Tx, did string) error
	ReadAll(ctx context.Context, tx *sqlx.Tx, did string) ([]SigningTaskData, error)
	ReadAllBySigner(ctx context.Context, tx *sqlx.Tx, signer string) ([]SigningTaskData, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, signer string, state string) error
	AnyTasksInState(ctx context.Context, tx *sqlx.Tx, did string, states ...string) (bool, error)
	IsValidSigner(ctx context.Context, tx *sqlx.Tx, did string, signer string) (bool, error)
	TaskExistsInState(ctx context.Context, tx *sqlx.Tx, did string, signer string, state string) (bool, error)
	TaskExists(ctx context.Context, tx *sqlx.Tx, did string) (bool, error)
	Delete(ctx context.Context, tx *sqlx.Tx, did string) error
}
