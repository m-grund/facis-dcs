package contractworkflowengine

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/conf"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	database "digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

type CronJob struct {
	DB    *sqlx.DB
	CRepo database.ContractRepo
}

func (j CronJob) Start(ctx context.Context, db *sqlx.DB) {
	go startExpiryScheduler(ctx, db, j.CRepo, conf.ExpirationCronJobTimeOut())
}

func startExpiryScheduler(ctx context.Context, db *sqlx.DB, repo database.ContractRepo, interval time.Duration) {

	readExpiredContracts := func() ([]database.ContractMetadata, error) {
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("could not start transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			err := tx.Rollback()
			if err != nil {
				log.Println("could not rollback transaction")
			}
		}(tx)

		expiredContracts, err := repo.ReadExpiredContacts(ctx, tx)
		if err != nil {
			return nil, fmt.Errorf("could not read expired contracts: %w", err)
		}

		err = tx.Commit()
		if err != nil {
			return nil, fmt.Errorf("could not commit transaction: %w", err)
		}

		return expiredContracts, nil
	}

	callExpirationLogic := func(expiredContract database.ContractMetadata) error {

		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			return fmt.Errorf("could not start transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			err := tx.Rollback()
			if err != nil {
				log.Println("could not rollback transaction")
			}
		}(tx)

		var policy *expirationpolicy.ExpirationPolicy
		if expiredContract.ExpPolicy != nil {
			p, err := expirationpolicy.NewExpirationPolicy(*expiredContract.ExpPolicy)
			if err != nil {
				return fmt.Errorf("could not create expiration policy: %w", err)
			}
			policy = &p
		} else {
			return fmt.Errorf("unknown expiration policy for expired contract with DID %s", expiredContract.DID)
		}

		err = repo.UpdateState(ctx, tx, expiredContract.DID, contractstate.Expired.String())
		if err != nil {
			return fmt.Errorf("could not update expired contract with DID %s: %w", expiredContract.DID, err)
		}

		state, err := contractstate.NewContractState(expiredContract.State)
		if err != nil {
			return fmt.Errorf("could not create state for expired contract with DID %s: %w", expiredContract.DID, err)
		}

		evt := contractevents.ContractExpired{
			DID:             expiredContract.DID,
			ContractVersion: expiredContract.ContractVersion,
			State:           state,
			ExpPolicy:       policy,
			OccurredAt:      time.Now().UTC(),
		}
		err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
		if err != nil {
			return fmt.Errorf("could not create event: %w", err)
		}

		switch *policy {
		case expirationpolicy.Renewal:
			fmt.Printf("ToDo: call renewal logic for expired contract with DID %s\n", expiredContract.DID)
		case expirationpolicy.Archiving:
			fmt.Printf("ToDo: call archiving logic for expired contract with DID %s\n", expiredContract.DID)
		case expirationpolicy.Termination:
			fmt.Printf("ToDo: call termination logic for expired contract with DID %s\n", expiredContract.DID)
		}

		return tx.Commit()
	}

	ticker := time.NewTicker(interval)
	for range ticker.C {

		expiredContracts, err := readExpiredContracts()
		if err != nil {
			log.Printf("could not read expired contracts: %v", err)
			continue
		}

		if len(expiredContracts) > 0 {
			log.Printf("%d contracts expired", len(expiredContracts))
		}

		for _, expiredContract := range expiredContracts {
			err = callExpirationLogic(expiredContract)
			if err != nil {
				log.Printf("could not call expiration logic for expired contract: %v", err)
			}
		}
	}
}
