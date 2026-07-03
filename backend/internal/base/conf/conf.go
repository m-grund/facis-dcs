// Package conf centralizes timing/topic configuration shared across domains
// (transaction timeouts, outbox/cron polling intervals, the event-bus topic
// name), so these values are changed in one place rather than duplicated
// per domain.
package conf

import (
	"time"
)

func TransactionTimeout() time.Duration {
	return 1 * time.Minute
}

func HTTPClientTimeout() time.Duration {
	return 1 * time.Minute
}

func OutboxProcessorTimeOut() time.Duration {
	return 1 * time.Second
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
