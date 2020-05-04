package ratelimit

import (
	"time"
)

// A Rate describes the request rate a user is limited to.
type Rate struct {
	Limit  int64         // max number of requests for a time period
	Period time.Duration // duration in which the limit applies
}
