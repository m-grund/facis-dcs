package base

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/db"
	"digital-contracting-service/internal/base/ipfs"

	"golang.org/x/sync/errgroup"
)

type AuditTrailReader struct {
	ARepo      db.AuditTrailRepository
	IPFSClient *ipfs.APIClient
}

func (r AuditTrailReader) ReadAuditLogEntriesByComponentAndDID(ctx context.Context, tx *sqlx.Tx, componentType componenttype.ComponentType, did string) ([]datatype.AuditLogEntry, error) {

	cid, err := r.ARepo.ReadLogCID(ctx, tx, componentType.String(), did)
	if err != nil {
		return nil, err
	}

	logEntries := make([]datatype.AuditLogEntry, 0)
	if cid == nil {
		return logEntries, nil
	}

	currentCID := *cid
	for {
		result, err := r.IPFSClient.FetchFile(currentCID)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
		var signedAuditLogEntry datatype.SignedAuditLogEntry
		if err := json.Unmarshal(result.Data, &signedAuditLogEntry); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}

		logEntries = append(logEntries, signedAuditLogEntry.AuditLogEntry)

		if signedAuditLogEntry.AuditLogEntry.ResLogPredCID == nil {
			break
		}

		currentCID = *signedAuditLogEntry.AuditLogEntry.ResLogPredCID
	}

	return logEntries, nil
}

func (r AuditTrailReader) ReadAuditLogEntriesByComponent(ctx context.Context, tx *sqlx.Tx, componentType componenttype.ComponentType) ([][]datatype.AuditLogEntry, error) {

	cids, err := r.ARepo.ReadLogCIDs(ctx, tx, componentType.String())
	if err != nil {
		return nil, err
	}

	// Each per-DID chain must be walked sequentially (entries link to their
	// predecessor by CID), but the chains are independent of each other —
	// walk them concurrently. A component-wide audit over N resources is
	// otherwise O(total events) sequential IPFS round-trips and blows past
	// any sane request deadline once the trail has real history.
	nonNil := make([]string, 0, len(cids))
	for _, cid := range cids {
		if cid != nil {
			nonNil = append(nonNil, *cid)
		}
	}

	chains := make([][]datatype.AuditLogEntry, len(nonNil))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(128)
	for i, headCID := range nonNil {
		g.Go(func() error {
			logEntries := make([]datatype.AuditLogEntry, 0)
			currentCID := headCID
			for {
				if err := gctx.Err(); err != nil {
					return err
				}
				fetched, err := r.IPFSClient.FetchFile(currentCID)
				if err != nil {
					return fmt.Errorf("read body: %w", err)
				}
				logEntry, err := decodeSignedAuditLogEntry(fetched.Data)
				if err != nil {
					return fmt.Errorf("decode response: %w", err)
				}

				logEntries = append(logEntries, logEntry)

				if logEntry.ResLogPredCID == nil {
					break
				}

				currentCID = *logEntry.ResLogPredCID
			}
			chains[i] = logEntries
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return chains, nil
}

func (r AuditTrailReader) ReadAllAuditLogEntries(ctx context.Context, tx *sqlx.Tx) ([]datatype.AuditLogEntry, error) {

	cid, err := r.ARepo.ReadLogCID(ctx, tx, conf.GlobalAuditTrailName(), conf.GlobalAuditTrailName())
	if err != nil {
		return nil, err
	}

	logEntries := make([]datatype.AuditLogEntry, 0)
	if cid == nil {
		return logEntries, nil
	}

	currentCID := *cid
	for {
		bodyBytes, err := r.IPFSClient.FetchFile(currentCID)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
		logEntry, err := decodeSignedAuditLogEntry(bodyBytes.Data)
		if err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}

		logEntries = append(logEntries, logEntry)

		if logEntry.GlobalLogPredCID == nil {
			break
		}

		currentCID = *logEntry.GlobalLogPredCID
	}

	return logEntries, nil
}

func decodeSignedAuditLogEntry(data []byte) (datatype.AuditLogEntry, error) {
	var signedAuditLogEntry datatype.SignedAuditLogEntry
	if err := json.Unmarshal(data, &signedAuditLogEntry); err != nil {
		return datatype.AuditLogEntry{}, err
	}
	return signedAuditLogEntry.AuditLogEntry, nil
}
