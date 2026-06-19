package config

import "time"

type Config struct {
	BASE_URL             string
	TIMEOUT_MS           time.Duration
	MAX_RETRIES          int
	MAX_POLL_INTERVAL_MS time.Duration
	MAX_POLL_DURATION_MS time.Duration
	MAX_POLL_ATTEMPTS    int
	SDK_ACTIONS
}

type SDK_ACTIONS struct {
	COLLECT_PAYMENT             string
	COLLECT_PAYMENT_AND_RESOLVE string
	MAKE_PAYOUT                 string
	MAKE_PAYOUT_AND_RESOLVE     string
	GET_STATUS                  string
	GET_TRANSACTION             string
	VERIFY_PHONE                string
	CREATE_INVOICE              string
}
