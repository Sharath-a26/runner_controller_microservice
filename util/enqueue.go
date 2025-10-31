package util

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func EnqueueRunRequest(ctx context.Context, runID string, fileName string, extension string) error {
	var logger = NewLogger()

	// Message represents the structure of our message
	type Message struct {
		RunId     string    `json:"runId"`
		FileName  string    `json:"fileName"`
		Extension string    `json:"extension"`
		Timestamp time.Time `json:"timestamp"`
	}

	// Declare a queue
	queueName := os.Getenv("REDIS_QUEUE_NAME")
	if queueName == "" {
		queueName = "task_queue"
		logger.Warn(fmt.Sprintf("REDIS_QUEUE_NAME not set, using default: %s", queueName))
	}

	// Create a new message
	msg := Message{
		RunId:     runID,
		FileName:  fileName,
		Extension: extension,
		Timestamp: time.Now(),
	}

	// Convert message to JSON
	body, err := json.Marshal(msg)
	if err != nil {
		logger.Error(fmt.Sprintf("Error marshaling message: %v", err))
		return err
	}

	// Push message to Redis List (LPUSH = enqueue at head)
	err = RedisClient.LPush(ctx, queueName, string(body)).Err();

	if err != nil {
		logger.Error(fmt.Sprintf("Failed to publish message: %v", err))
		return err
	}

	logger.Info(fmt.Sprintf("Published message: %s", msg.RunId))
	return nil
}
