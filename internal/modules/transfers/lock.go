package transfers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

const defaultProcessingLockPrefix = "aegis:transfers:processing:"

var releaseProcessingLockScript = goredis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

type ProcessingLocker interface {
	Acquire(ctx context.Context, transferID string, ttl time.Duration) (ProcessingLock, bool, error)
}

type ProcessingLock interface {
	Release(ctx context.Context) error
}

type RedisProcessingLocker struct {
	client *goredis.Client
	prefix string
}

type redisProcessingLock struct {
	client *goredis.Client
	key    string
	token  string
}

func NewRedisProcessingLocker(client *goredis.Client, prefix string) *RedisProcessingLocker {
	if prefix == "" {
		prefix = defaultProcessingLockPrefix
	}

	return &RedisProcessingLocker{
		client: client,
		prefix: prefix,
	}
}

func (l *RedisProcessingLocker) Acquire(ctx context.Context, transferID string, ttl time.Duration) (ProcessingLock, bool, error) {
	if l == nil || l.client == nil {
		return nil, true, nil
	}

	if ttl <= 0 {
		ttl = 30 * time.Second
	}

	key := l.prefix + transferID
	token := uuid.NewString()

	acquired, err := l.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return nil, false, fmt.Errorf("acquire redis processing lock: %w", err)
	}

	if !acquired {
		return nil, false, nil
	}

	return &redisProcessingLock{
		client: l.client,
		key:    key,
		token:  token,
	}, true, nil
}

func (l *redisProcessingLock) Release(ctx context.Context) error {
	if l == nil || l.client == nil {
		return nil
	}

	if _, err := releaseProcessingLockScript.Run(ctx, l.client, []string{l.key}, l.token).Result(); err != nil {
		return fmt.Errorf("release redis processing lock: %w", err)
	}

	return nil
}
