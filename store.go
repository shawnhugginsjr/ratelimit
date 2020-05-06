package ratelimit

import (
	"context"
)

// Store is the common interface for limiter stores.
type Store interface {
	// RecordRequest will increment the request count for the key before returning
	// a LimitRecord reflecting the new count.
	RecordRequest(ctx context.Context, key string, rate Rate) (LimitRecord, error)
	// CheckLimit returns the LimitRecord for the key without increasing the request
	// count.
	CheckLimit(ctx context.Context, key string, rate Rate) (LimitRecord, error)
}

// StoreOptions are options for store.
type StoreOptions struct {
	Prefix     string // prefix to use for a key
	RetryLimit int    // max number of retries during race conditions
}
