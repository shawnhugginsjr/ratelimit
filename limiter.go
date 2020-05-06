package ratelimit

import (
	"context"
	"time"
)

// The LimitRecord stores data concerning a request limit. The fields
// are based from the X-Rate-Limit-Limit, X-Rate-Limit-Remaining, and
// X-Rate-Limit-Reset headers.
type LimitRecord struct {
	Limit        int64     // request limit per time window
	Remaining    int64     // remaining requests per window
	Reset        time.Time // expiration time of window
	LimitReached bool      // if the request limit has been reached
}

// NewLimitRecord returns a LimitRecord based from the paramters.
func NewLimitRecord(rate Rate, expiration time.Time, count int64) LimitRecord {
	LimitReached := true
	remaining := int64(0)

	if count <= rate.Limit {
		remaining = rate.Limit - count
		LimitReached = false
	}

	return LimitRecord{
		Limit:        rate.Limit,
		Remaining:    remaining,
		Reset:        expiration,
		LimitReached: LimitReached,
	}
}

// SecondsRemaining returns the remaining seconds of a Limit.
func (lc *LimitRecord) SecondsRemaining() int64 {
	secondsRemaining := int64(lc.Reset.Sub(time.Now()).Seconds())
	if secondsRemaining < 0 {
		secondsRemaining = 0
	}
	return secondsRemaining
}

// The Limiter rate limits an IPAdress
type Limiter struct {
	Store Store // Store managing rate limits
	Rate  Rate  //  Rate to use for this Limmiter
}

func (l *Limiter) RecordRequest(ctx context.Context, key string) (LimitRecord, error) {
	return l.Store.RecordRequest(ctx, key, l.Rate)
}

func (l *Limiter) CheckLimit(ctx context.Context, key string) (LimitRecord, error) {
	return l.Store.CheckLimit(ctx, key, l.Rate)
}
