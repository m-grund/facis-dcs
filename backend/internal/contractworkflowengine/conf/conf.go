package conf

import "time"

func ExpirationCronJobTimeOut() time.Duration {
	return 1 * time.Second
}
