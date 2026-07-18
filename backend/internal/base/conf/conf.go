// Package conf centralizes timing/topic configuration shared across domains
// (transaction timeouts, outbox/cron polling intervals, the event-bus topic
// name), so these values are changed in one place rather than duplicated
// per domain.
package conf

import (
	"os"
	"time"
)

func TransactionTimeout() time.Duration {
	return 1 * time.Minute
}

// SystemToken is the in-cluster service credential the background PDF
// regenerator presents to the internal signing primitives (it runs on NATS
// events with no user JWT). Empty when unset — no system caller is accepted.
func SystemToken() string {
	return os.Getenv("DCS_SYSTEM_TOKEN")
}

func HTTPClientTimeout() time.Duration {
	return 1 * time.Minute
}

func OutboxProcessorTimeOut() time.Duration {
	return 1 * time.Second
}

// OutboxPublishTimeOut is the poll interval for republishing outbox events
// on NATS (see event.OutboxProcessor.startPublishingJob): much tighter than
// OutboxProcessorTimeOut because publishing is a cheap, single NATS call per
// event, unlike the TSA/IPFS round-trips the (slower) anchoring loop does.
func OutboxPublishTimeOut() time.Duration {
	return 100 * time.Millisecond
}

func EventBusTopic() string {
	return "dcs"
}

func GlobalAuditTrailName() string {
	return "GLOBAL_AUDIT_TRAIL"
}

func LoginAttemptsThresholdInDuration() int {
	return 5
}

func LoginLockoutDuration() time.Duration {
	return 15 * time.Minute
}

func SyncFailCronJobTimeOut() time.Duration {
	return 24 * time.Hour
}
