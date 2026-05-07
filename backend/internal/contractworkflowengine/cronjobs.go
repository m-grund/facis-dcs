package contractworkflowengine

import (
	"context"
	"digital-contracting-service/internal/contractworkflowengine/conf"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
)

type CronJob struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (j CronJob) Start(ctx context.Context, db *sqlx.DB) {
	go startExpiryScheduler(ctx, db, j.CRepo, conf.ExpirationCronJobTimeOut())
}

func startExpiryScheduler(ctx context.Context, db *sqlx.DB, repo db.ContractRepo, interval time.Duration) {

	schedulerLogic := func() error {
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			return fmt.Errorf("could not start transaction: %w", err)
		}
		defer tx.Rollback()

		affected, err := repo.ExpireOutdatedContracts(ctx, nil)
		if err != nil {
			return fmt.Errorf("could not set contract state to EXPIRED: %w", err)
		}
		log.Printf("%d contracts expried", affected)

		return tx.Commit()
	}

	ticker := time.NewTicker(interval)
	for range ticker.C {
		err := schedulerLogic()
		if err != nil {
			log.Printf("could not update contract states: %v", err)
		}
	}
}
