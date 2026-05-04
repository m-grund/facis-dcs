package base

import (
	"context"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/db"
	"digital-contracting-service/internal/base/ipfs"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
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
		var logEntry datatype.AuditLogEntry
		if err := json.Unmarshal(result.Data, &logEntry); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}

		logEntries = append(logEntries, logEntry)

		if logEntry.ResLogPredCID == nil {
			break
		}

		currentCID = *logEntry.ResLogPredCID
	}

	return logEntries, nil
}

func (r AuditTrailReader) ReadAuditLogEntriesByComponent(ctx context.Context, tx *sqlx.Tx, componentType componenttype.ComponentType) ([][]datatype.AuditLogEntry, error) {

	cids, err := r.ARepo.ReadLogCIDs(ctx, tx, componentType.String())
	if err != nil {
		return nil, err
	}

	result := make([][]datatype.AuditLogEntry, 0)
	for _, cid := range cids {
		logEntries := make([]datatype.AuditLogEntry, 0)
		if cid == nil {
			return nil, nil
		}

		currentCID := *cid
		for {
			result, err := r.IPFSClient.FetchFile(currentCID)
			if err != nil {
				return nil, fmt.Errorf("read body: %w", err)
			}
			var logEntry datatype.AuditLogEntry
			if err := json.Unmarshal(result.Data, &logEntry); err != nil {
				return nil, fmt.Errorf("decode response: %w", err)
			}

			logEntries = append(logEntries, logEntry)

			if logEntry.ResLogPredCID == nil {
				break
			}

			currentCID = *logEntry.ResLogPredCID
		}
	}

	return result, nil
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
		var logEntry datatype.AuditLogEntry
		if err := json.Unmarshal(bodyBytes.Data, &logEntry); err != nil {
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
