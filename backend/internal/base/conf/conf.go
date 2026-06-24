package conf

import (
	"fmt"
	"strings"
	"time"
)

func TransactionTimeout() time.Duration {
	return 5 * time.Minute
}

func OutboxProcessorTimeOut() time.Duration {
	return 1 * time.Second
}

func EventBusTopic(subTopicName string) string {
	namespace := "dcs"
	if subTopicName == "" || subTopicName == "*" {
		fmt.Sprintf("%s.*", namespace)
	}
	return fmt.Sprintf("%s.%s", namespace, strings.ToLower(subTopicName))
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
