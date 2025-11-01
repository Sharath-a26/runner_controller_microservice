package util

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func EnqueueRunRequest(ctx context.Context, runID string, fileName string, extension string) error {
	var logger = SharedLogger

	// Message represents the structure of our message
	type Message struct {
		RunId     string    `json:"runId"`
		FileName  string    `json:"fileName"`
		Extension string    `json:"extension"`
		Timestamp time.Time `json:"timestamp"`
	}

	// Get RabbitMQ connection string from environment variable or use default
	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		rabbitMQURL = "amqp://guest:guest@localhost:5672/"
	}

	// Connect to RabbitMQ server
	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to connect to RabbitMQ: %v", err), err)
	}
	defer conn.Close()

	// Create a channel
	ch, err := conn.Channel()
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to open a channel: %v", err), err)
	}
	defer ch.Close()

	// Declare a queue
	queueName := "task_queue"
	q, err := ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)

	if err != nil {
		logger.Error(fmt.Sprintf("Failed to declare a queue: %v", err), err)
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
		logger.Error(fmt.Sprintf("Error marshaling message: %v", err), err)
    return err
	}

	// Push message to Redis List (LPUSH = enqueue at head)
	err = RedisClient.LPush(ctx, queueName, string(body)).Err();

	if err != nil {
		logger.Error(fmt.Sprintf("Failed to publish message: %v", err), err)
		return err
	}

	logger.Info(fmt.Sprintf("Published message: %s", msg.RunId))
	return nil
}
