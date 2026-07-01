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
	return 60 * time.Minute
}
