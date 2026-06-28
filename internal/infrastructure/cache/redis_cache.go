package cache

import (
	"context"
	"time"

	domain_cache "aiProject/internal/domain/cache"
	"aiProject/internal/shared"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// keyPrefix 本应用在 Redis 中使用的统一命名空间前缀，
// Clear 仅清理该前缀下的 key，避免误删共享 Redis 中的其它数据。
const keyPrefix = "aiproject:cache:"

// RedisCache 基于 Redis 的缓存实现。
type RedisCache struct {
	client    *redis.Client
	available bool
}

// RedisOptions Redis 连接参数。
type RedisOptions struct {
	Addr     string        // 地址，如 localhost:6379
	Password string        // 密码（来自环境变量，可为空）
	DB       int           // 数据库编号
	Timeout  time.Duration // 连接/读写超时
}

// NewRedisCache 创建 Redis 缓存。连接探测失败时返回 available=false 的实例，
// 调用方可据此降级（缓存读写均变为 no-op 效果），不影响主流程。
func NewRedisCache(opts RedisOptions) *RedisCache {
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Second
	}
	client := redis.NewClient(&redis.Options{
		Addr:         opts.Addr,
		Password:     opts.Password,
		DB:           opts.DB,
		DialTimeout:  opts.Timeout,
		ReadTimeout:  opts.Timeout,
		WriteTimeout: opts.Timeout,
	})

	rc := &RedisCache{client: client}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		shared.GetLogger().Warn("Redis 连接失败，缓存将降级为不可用状态",
			zap.String("addr", opts.Addr), zap.Error(err))
		rc.available = false
		return rc
	}
	rc.available = true
	shared.GetLogger().Info("Redis 缓存已连接", zap.String("addr", opts.Addr), zap.Int("db", opts.DB))
	return rc
}

// fullKey 给业务 key 加上应用命名空间前缀。
func fullKey(key string) string {
	return keyPrefix + key
}

// Get 读取缓存。
func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if !c.available {
		return nil, false, nil
	}
	val, err := c.client.Get(ctx, fullKey(key)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}

// Set 写入缓存，ttl<=0 表示永不过期。
func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if !c.available {
		return nil
	}
	if ttl < 0 {
		ttl = 0
	}
	return c.client.Set(ctx, fullKey(key), value, ttl).Err()
}

// Delete 删除指定 key。
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	if !c.available {
		return nil
	}
	return c.client.Del(ctx, fullKey(key)).Err()
}

// Size 返回本应用命名空间下的 key 数量（通过 SCAN 统计，避免阻塞）。
func (c *RedisCache) Size(ctx context.Context) (int64, error) {
	if !c.available {
		return 0, nil
	}
	var count int64
	var cursor uint64
	pattern := keyPrefix + "*"
	for {
		keys, next, err := c.client.Scan(ctx, cursor, pattern, 256).Result()
		if err != nil {
			return count, err
		}
		count += int64(len(keys))
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return count, nil
}

// Clear 清空本应用命名空间下的所有 key（通过 SCAN + 批量删除）。
func (c *RedisCache) Clear(ctx context.Context) error {
	if !c.available {
		return nil
	}
	var cursor uint64
	pattern := keyPrefix + "*"
	for {
		keys, next, err := c.client.Scan(ctx, cursor, pattern, 256).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}

// Available 后端是否可用。
func (c *RedisCache) Available() bool { return c.available }

// Backend 返回后端标识。
func (c *RedisCache) Backend() string { return "redis" }

// Close 关闭连接。
func (c *RedisCache) Close() error {
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}

// 确保实现了领域接口
var _ domain_cache.Cache = (*RedisCache)(nil)
