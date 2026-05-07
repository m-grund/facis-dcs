package contractworkflowengine

import (
	"context"
	"digital-contracting-service/internal/contractworkflowengine/conf"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
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

		expiredContracts, err := repo.ReadExpiredContacts(ctx, tx)
		if err != nil {
			return fmt.Errorf("could not read expired contracts: %w", err)
		}

		if len(expiredContracts) > 0 {
			log.Printf("%d contracts expired", len(expiredContracts))
		}

		for _, expiredContract := range expiredContracts {

			var policy *expirationpolicy.ExpirationPolicy
			if expiredContract.ExpPolicy != nil {
				p, err := expirationpolicy.NewExpirationPolicy(*expiredContract.ExpPolicy)
				if err != nil {
					fmt.Errorf("could not create expiration policy: %w", err)
					continue
				}
				policy = &p
			} else {
				fmt.Errorf("unknown expiration policy for expired contract with DID %s\n", expiredContract.DID)
				continue
			}

			switch *policy {
			case expirationpolicy.Renewal:
				fmt.Printf("ToDo: call renewal logic for expired contract with DID %s\n", expiredContract.DID)
			case expirationpolicy.Archiving:
				fmt.Printf("ToDo: call archiving logic for expired contract with DID %s\n", expiredContract.DID)
			case expirationpolicy.Termination:
				fmt.Printf("ToDo: call termination logic for expired contract with DID %s\n", expiredContract.DID)
			}

		}

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
