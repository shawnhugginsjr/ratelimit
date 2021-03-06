package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnhugginsjr/ratelimit"

	redisClient "github.com/go-redis/redis/v7"
	"github.com/pkg/errors"
)

// The Client interface communicates to the redis server(s). This allows support
// for both a redis client and redis cluster client.
type Client interface {
	Get(key string) *redisClient.StringCmd
	Set(key string, value interface{}, expiration time.Duration) *redisClient.StatusCmd
	SetNX(key string, value interface{}, expiration time.Duration) *redisClient.BoolCmd
	Del(keys ...string) *redisClient.IntCmd
	Watch(handler func(*redisClient.Tx) error, keys ...string) error
	Eval(script string, keys []string, args ...interface{}) *redisClient.Cmd
}

// Store is the redis store.
type Store struct {
	Prefix     string // Prefix used for the key.
	RetryLimit int    // RetryLimit is the maximum number of retry under race conditions.
	Client     Client // client used to communicate with redis server.
}

// RecordRequest will increment the visit count for a specific ID.
func (store *Store) RecordRequest(ctx context.Context, key string, rate ratelimit.Rate) (ratelimit.LimitRecord, error) {
	key = fmt.Sprintf("%s:%s", store.Prefix, key)
	var lr ratelimit.LimitRecord
	onWatch := func(tx *redisClient.Tx) error {
		created, err := store.trySetNX(tx, key, rate.Period)
		if err != nil {
			return err
		}

		if created {
			expiration := time.Now().Add(rate.Period)
			lr = ratelimit.NewLimitRecord(rate, expiration, 1)
			return nil
		}

		count, ttl, err := store.tryIncrementValue(tx, key, rate.Period)
		if err != nil {
			return err
		}

		now := time.Now()
		expiration := now.Add(rate.Period)
		if ttl > 0 {
			expiration = now.Add(ttl)
		}

		lr = ratelimit.NewLimitRecord(rate, expiration, count)
		return nil
	}

	err := store.Client.Watch(onWatch, key)
	if err != nil {
		err = errors.Wrapf(err, "ratelimit: cannot get value for %s", key)
		return ratelimit.LimitRecord{}, err
	}

	return lr, nil
}

// trySetNX will attempt to execute setNX once within a retry limit. There is a race condition
// where multiple requests try to update the visit count at the same time, so optimistic locking
// is used to resolve the issue.
func (store *Store) trySetNX(tx *redisClient.Tx, key string, expiration time.Duration) (bool, error) {
	for i := 0; i < store.RetryLimit; i++ {
		created, err := setNX(tx, key, expiration)
		if err == nil {
			return created, nil
		}
	}
	return false, errors.New("retry limit exceeded")
}

// setNX will init a counter if the key does not exist.
func setNX(tx *redisClient.Tx, key string, expiration time.Duration) (bool, error) {
	value := tx.SetNX(key, 1, expiration)

	created, err := value.Result()
	if err != nil {
		return false, err
	}

	return created, nil
}

// tryIncrementValue will attempt to execute incrementValue once within a retry limit. There is a race
// condition where multiple requests try to update the visit count at the same time, so optimistic locking
// is used to resolve the issue.
func (store *Store) tryIncrementValue(tx *redisClient.Tx, key string,
	expiration time.Duration) (int64, time.Duration, error) {
	for i := 0; i < store.RetryLimit; i++ {
		count, ttl, err := incrementValue(tx, key, expiration)
		if err == nil {
			return count, ttl, nil
		}

		// If ttl is negative and there is an error, do not retry an update.
		if ttl < 0 {
			return 0, 0, err
		}
	}
	return 0, 0, errors.New("retry limit exceeded")
}

// incrementValue will increment the counter identified by given key.
func incrementValue(tx *redisClient.Tx, key string, expiration time.Duration) (int64, time.Duration, error) {
	pipe := tx.TxPipeline()
	value := pipe.Incr(key)
	expire := pipe.PTTL(key)

	_, err := pipe.Exec()
	if err != nil {
		return 0, 0, err
	}

	count, err := value.Result()
	if err != nil {
		return 0, 0, err
	}

	keyTTL, err := expire.Result()
	if err != nil {
		return 0, 0, err
	}

	// The PTTL command returns -2 if the key does not exist, and -1 if the key exists, but there is no expiry set.
	// We shouldn't try to set an expiry on a key that doesn't exist.
	if isExpirationRequired(keyTTL) {
		expire := tx.Expire(key, expiration)

		ok, err := expire.Result()
		if err != nil {
			return count, keyTTL, err
		}

		if !ok {
			return count, keyTTL, errors.New("cannot set timeout for key")
		}
	}

	return count, keyTTL, nil
}

func isExpirationRequired(ttl time.Duration) bool {
	switch ttl {
	case -1 * time.Nanosecond, -1 * time.Millisecond:
		return true
	default:
		return false
	}
}

// CheckLimit returns the limit for a given identifier without chaning the count.
func (store *Store) CheckLimit(ctx context.Context, key string, rate ratelimit.Rate) (ratelimit.LimitRecord, error) {
	key = fmt.Sprintf("%s:%s", store.Prefix, key)

	lr := ratelimit.LimitRecord{}
	onWatch := func(tx *redisClient.Tx) error {
		count, ttl, err := store.tryCheckLimit(tx, key)
		if err != nil {
			return errors.Wrap(err, "store: tryCheckValue failed")
		}

		now := time.Now()
		expiration := now.Add(rate.Period)
		if ttl > 0 {
			expiration = now.Add(ttl)
		}

		lr = ratelimit.NewLimitRecord(rate, expiration, count)
		return nil
	}

	err := store.Client.Watch(onWatch, key)
	if err != nil {
		err = errors.Wrapf(err, "redis-store: cannot check limit for %s", key)
		return lr, err
	}

	return lr, nil
}

// tryCheckLimit will attempt to execute checkLimit once within a retry limit. There is a race
// condition where multiple requests try to update the visit count at the same time, so optimistic locking
// is used to resolve the issue.
func (store *Store) tryCheckLimit(tx *redisClient.Tx, key string) (int64, time.Duration, error) {
	for i := 0; i < store.RetryLimit; i++ {
		count, ttl, err := checkLimit(tx, key)
		if err == nil {
			return count, ttl, nil
		}
	}
	return 0, 0, errors.New("redis-store: retry limit exceeded")
}

// checkLimit will retrieve the counter and its expiration for given key.
func checkLimit(tx *redisClient.Tx, key string) (int64, time.Duration, error) {
	pipe := tx.TxPipeline()
	value := pipe.Get(key)
	expire := pipe.PTTL(key)

	_, err := pipe.Exec()
	if err != nil && err != redisClient.Nil {
		return 0, 0, errors.Wrapf(err, "redis-store: pipelined commands failed for key %s", key)
	}

	count, err := value.Int64()
	if err != nil && err != redisClient.Nil {
		return 0, 0, errors.Wrapf(err, "redis-store: count could not be retrieved for key %s", key)
	}

	ttl, err := expire.Result()
	if err != nil {
		return 0, 0, errors.Wrapf(err, "redis-store: count ttl could not be retrieved for key %s", key)
	}

	return count, ttl, nil
}
