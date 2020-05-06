package redis_test

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	redislib "github.com/go-redis/redis/v7"
	"github.com/shawnhugginsjr/ratelimit"
	"github.com/shawnhugginsjr/ratelimit/stores/redis"
)

func TestIDTracking(t *testing.T) {
	client := NewRedisClient()
	ctx := context.Background()
	key := "TestIDTracking"
	store := redis.Store{
		Prefix:     "test",
		RetryLimit: 3,
		Client:     client,
	}
	rate := ratelimit.Rate{
		Limit:  3,
		Period: 1 * time.Second,
	}

	for i := int64(0); i < rate.Limit; i++ {
		requestNumber := i + 1
		testName := fmt.Sprintf("Track Request %d", requestNumber)
		t.Run(testName, func(t *testing.T) {
			lr, err := store.RecordRequest(ctx, key, rate)
			if err != nil {
				t.Error(err.Error())
			}

			if lr.Remaining != lr.Limit-requestNumber {
				t.Errorf("Expected %d remaining request(s), not %d", lr.Limit-requestNumber, lr.Remaining)
			}

		})
	}

	t.Run("RequestLimitReached", func(t *testing.T) {
		lr, err := store.RecordRequest(ctx, key, rate)
		if err != nil {
			t.Error(err.Error())
		}

		if lr.LimitReached == false {
			t.Error("Expected LimitedReached to be true, not false")
		}
	})

	t.Run("TestExpiration", func(t *testing.T) {
		fullKey := store.Prefix + ":" + key
		time.Sleep(1 * time.Second)
		val := client.Get(fullKey).Val()
		if val != "" {
			t.Errorf("Key '%s' still exists after expiration time", fullKey)
		}

	})
}

func TestConcurrentAccess(t *testing.T) {
	client := NewRedisClient()
	ctx := context.Background()
	key := "concurrentacess"
	store := redis.Store{
		Prefix:     "test",
		RetryLimit: 3,
		Client:     client,
	}
	rate := ratelimit.Rate{
		Limit:  100000000,
		Period: 10 * time.Second,
	}

	goroutines := 100
	ops := 500
	wg := &sync.WaitGroup{}
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			for j := 0; j < ops; j++ {
				_, err := store.RecordRequest(ctx, key, rate)
				if err != nil {
					t.Error(err.Error())
				}
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	val, err := strconv.Atoi(client.Get(store.Prefix + ":" + key).Val())
	if err != nil {
		t.Error(err.Error())
	}
	testCount := int64(val)
	expectedCount := int64(goroutines * ops)
	if testCount != expectedCount {
		t.Errorf("Counted %d requests instead of %d", testCount, expectedCount)
	}
}

func NewRedisClient() *redislib.Client {
	client := redislib.NewClient(&redislib.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	return client
}
