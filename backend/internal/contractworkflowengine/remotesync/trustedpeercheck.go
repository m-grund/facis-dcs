package remotesync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"digital-contracting-service/internal/dcstodcssynchronizer/db"

	"github.com/jmoiron/sqlx"
)

func CheckForUntrustedPeers(ctx context.Context, db *sqlx.DB, sRepo db.SyncRepository, localPeer string, responsible []string) ([]string, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	var untrustedPeers []string
	for _, peer := range responsible {
		if peer == localPeer {
			continue
		}

		trusted, err := sRepo.IsTrustedPeer(ctx, tx, peer)
		if err != nil {
			return nil, fmt.Errorf("could not check trusted peer: %w", err)
		}

		if !trusted {
			untrustedPeers = append(untrustedPeers, peer)
		}
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return untrustedPeers, nil
}
