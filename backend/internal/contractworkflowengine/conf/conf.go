package conf

import "time"

func ExpirationCronJobTimeOut() time.Duration {
	return 1 * time.Minute
}

func SyncFailCronJobTimeOut() time.Duration {
	return 10 * time.Second
}
