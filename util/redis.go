package util

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)
  
var RedisClient *redis.Client

// InitRedisClient initializes a Redis client.
func InitRedisClient(logger Logger) (error) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
		logger.Warn(fmt.Sprintf("REDIS_URL not set, using default: %s", redisURL))
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to parse REDIS_URL '%s': %v", redisURL, err))
		return err
	}
	rdb := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to connect to Redis at %s: %v", opts.Addr, err))
		return err
	}
	logger.Info(fmt.Sprintf("Successfully connected to Redis at %s", opts.Addr))
	RedisClient = rdb
	return nil
}

// ShutDownRedisClient shuts down the Redis client
func ShutDownRedisClient(logger Logger) {
	logger.Info("Shutting down Redis client...")
	if redisErr := RedisClient.Close(); redisErr != nil {
		logger.Error(fmt.Sprintf("Redis client shutdown error: %v", redisErr))
	} else {
		logger.Info("Redis client shutdown complete.")
	}
}