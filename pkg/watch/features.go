package watch

import (
	"os"
	"strconv"
)

const CheckLimitKey = "WM_CHECK_LIMIT"

var limitChecksEnabled = true

// Allows limit checks to be disabled for testing.
func SetLimitChecksEnabled(enabled bool) {
	limitChecksEnabled = enabled
}

func LimitChecksEnabled() bool {
	env, ok := os.LookupEnv(CheckLimitKey)
	if ok {
		enabled, err := strconv.ParseBool(env)
		if err == nil {
			return enabled
		}
	}

	return limitChecksEnabled
}
