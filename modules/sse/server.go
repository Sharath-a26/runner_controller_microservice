package sse

import (
	"context"
	"encoding/json"
	"errors"
	"evolve/util"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	runIdHeader     = "X-RUN-ID"      // Header key for the run ID.
	retrySeconds    = 3               // SSE retry interval suggestion for clients.
	sseDoneEvent    = "done"          // Event name for the end of the stream.
	eofStatus       = "EOF"           // Expected status value for the end message.
	logDataField    = "log_data"      // Field name in Redis Stream (must match 'runner').
	streamReadCount = 100             // How many messages to read per XREAD call.
	blockTimeout    = 5 * time.Second // Block timeout for XREAD waiting for new messages.
)

// redisLogPayload represents the structure
// of the log data from Redis.
type redisLogPayload struct {
	Stream string `json:"stream"` // stdout or stderr.
	Line   string `json:"line"`
	Status string `json:"status"` // Check for EOF status here.
	RunID  string `json:"runId"`  // From EOF message.
}

// GetSSEHandler returns an HTTP handler
// for Server-Sent Events (SSE) using Redis Streams.
func GetSSEHandler(logger util.Logger) http.HandlerFunc {
	if util.RedisClient == nil {
		logger.Error("GetSSEHandler requires a non-nil Redis client")
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Internal Server Error: Redis client not configured", http.StatusInternalServerError)
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		serveSSEWithStream(logger, w, r) // Call the new function
	}
}

func sendSSEData(w http.ResponseWriter, rc *http.ResponseController, payload string, runId string, logger *util.Logger) bool {
	// logger.Info(fmt.Sprintf("[SSE SENDING DATA] runId=%s | data=%s", runId, payload)) // Debug log
	_, writeErr := fmt.Fprintf(w, "data: %s\n\n", payload) // Payload should already be JSON string
	if writeErr != nil {
		// Don't log excessive errors if client simply disconnected
		if !errors.Is(writeErr, context.Canceled) && !strings.Contains(writeErr.Error(), "client disconnected") && !strings.Contains(writeErr.Error(), "connection reset by peer") {
			logger.Warn(fmt.Sprintf("[SSE WRITE ERROR] runId=%s | error=%v", runId, writeErr))
		}
		return false // Indicate failure
	}
	if flushErr := rc.Flush(); flushErr != nil {
		if !errors.Is(flushErr, context.Canceled) && !strings.Contains(flushErr.Error(), "client disconnected") {
			logger.Error(fmt.Sprintf("[SSE FLUSH ERROR] runId=%s | error=%v", runId, flushErr))
		}
		return false // Indicate failure
	}

	time.Sleep(256 * time.Millisecond)
	return true // Indicate success
}

// serveSSEWithStream handles the SSE stream for a given run ID.
func serveSSEWithStream(logger util.Logger, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger.Info("[SSE Stream Handler] Entered serveSSEWithStream")

	runId := r.URL.Query().Get("runId")
	if runId == "" {
		runId = r.Header.Get(runIdHeader)
		if runId == "" {
			logger.Warn(fmt.Sprintf("[SSE Stream Handler] Missing runId query parameter AND %s header", runIdHeader))
			http.Error(w, fmt.Sprintf("Missing runId query parameter or %s header", runIdHeader), http.StatusBadRequest)
			return
		}
		logger.Info(fmt.Sprintf("[SSE Stream Handler] Using runId from header: %s", runId))
	} else {
		logger.Info(fmt.Sprintf("[SSE Stream Handler] Using runId from query parameter: %s", runId))
	}

	// Basic Sanitize Run ID.
	if strings.ContainsAny(runId, "\n\r*?") {
		logger.Warn(fmt.Sprintf("[SSE Stream Handler] Invalid characters suspected in runId: %s", runId))
		http.Error(w, "Invalid Run ID format", http.StatusBadRequest)
		return
	}

	redisStreamName := runId
	logger.Info(fmt.Sprintf("[SSE Stream Handler] Determined runId: '%s', Stream Name: '%s'", runId, redisStreamName))

	// Set SSE Headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	_, err := fmt.Fprintf(w, "retry: %ds\n\n", retrySeconds)
	if err != nil {
		logger.Error(fmt.Sprintf("[SSE Stream Handler] Error writing retry header for runId %s: %v", runId, err))
		return
	}

	rc := http.NewResponseController(w)
	if rc == nil {
		logger.Error(fmt.Sprintf("[SSE Stream Handler] Failed to get ResponseController for runId: %s", runId))
		return
	}
	if err := rc.Flush(); err != nil {
		logger.Error(fmt.Sprintf("[SSE Stream Handler] Error flushing headers for runId %s: %v", runId, err))
		return
	}
	logger.Info(fmt.Sprintf("[SSE Stream Handler] Flushed SSE headers for runId: %s", runId))

	// sendSSEData := func(payload string) bool {
	// 	// logger.Info(fmt.Sprintf("[SSE SENDING DATA] runId=%s | data=%s", runId, payload)) // Debug log
	// 	_, writeErr := fmt.Fprintf(w, "data: %s\n\n", payload) // Payload should already be JSON string
	// 	if writeErr != nil {
	// 		// Don't log excessive errors if client simply disconnected
	// 		if !errors.Is(writeErr, context.Canceled) && !strings.Contains(writeErr.Error(), "client disconnected") && !strings.Contains(writeErr.Error(), "connection reset by peer") {
	// 			logger.Warn(fmt.Sprintf("[SSE WRITE ERROR] runId=%s | error=%v", runId, writeErr))
	// 		}
	// 		return false // Indicate failure
	// 	}
	// 	if flushErr := rc.Flush(); flushErr != nil {
	// 		if !errors.Is(flushErr, context.Canceled) && !strings.Contains(flushErr.Error(), "client disconnected") {
	// 			logger.Error(fmt.Sprintf("[SSE FLUSH ERROR] runId=%s | error=%v", runId, flushErr))
	// 		}
	// 		return false // Indicate failure
	// 	}

	// 	time.Sleep(256 * time.Millisecond)
	// 	return true // Indicate success
	// }

	// Process Stream.

	// Read History from beginning.
	lastProcessedID := "0-0"
	logger.Info(fmt.Sprintf("[SSE Stream Handler] Reading historical logs for stream: '%s' from ID: %s", redisStreamName, lastProcessedID))
	historyProcessed := 0

	for {
		// Use XRead to get batches of historical data
		// We don't block here, just read what's available.
		cmd := util.RedisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{redisStreamName, lastProcessedID},
			Count:   streamReadCount,
		})
		results, err := cmd.Result()

		if err != nil {
			// If stream doesn't exist yet, redis.Nil might not be returned by XRead.
			// Check error message for "NOGROUP" or similar if needed, but often just means empty.
			// For now, assume empty stream is not an error, just means no history.
			if errors.Is(err, redis.Nil) || strings.Contains(err.Error(), "NOGROUP") {
				logger.Info(fmt.Sprintf("[SSE Stream Handler] No historical logs found or reached end for stream: '%s'", redisStreamName))
				break
			} else if errors.Is(err, context.Canceled) {
				// Client disconnected.
				logger.Info(fmt.Sprintf("[SSE Stream Handler] Context cancelled during history read for runId: %s", runId))
				return
			}
			logger.Error(fmt.Sprintf("[SSE Stream Handler] Error reading history from stream '%s': %v", redisStreamName, err))
			return
		}

		if len(results) == 0 || len(results[0].Messages) == 0 {
			// logger.Info(fmt.Sprintf("[SSE Stream Handler] Finished reading history batch for stream: '%s'", redisStreamName))
			break
		}

		streamMessages := results[0].Messages
		logger.Info(fmt.Sprintf("[SSE Stream Handler] Processing %d historical messages for stream: '%s'", len(streamMessages), redisStreamName))

		for _, msg := range streamMessages {
			logPayloadStr, ok := msg.Values[logDataField].(string)
			if !ok {
				logger.Warn(fmt.Sprintf("[SSE Stream Handler] Invalid data format in stream '%s', ID '%s': Missing or non-string field '%s'", redisStreamName, msg.ID, logDataField))
				continue
			}

			// Send the payload.
			if !sendSSEData(w, rc, logPayloadStr, runId, &logger) {
				return
			}
			historyProcessed++

			// Check if this message is the EOF marker.
			var logData redisLogPayload
			if json.Unmarshal([]byte(logPayloadStr), &logData) == nil && logData.Status == eofStatus {
				logger.Info(fmt.Sprintf("[SSE Stream Handler] EOF marker found in history (ID: %s) for runId: %s", msg.ID, runId))
				// Send the done event *now* and finish.
				doneData := `{"message": "Stream ended (found in history)."}`
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", sseDoneEvent, doneData)
				_ = rc.Flush()
				return
			}
			// Update last ID processed.
			lastProcessedID = msg.ID
		}

		// If we read less than requested count, we are likely at the end of history for now.
		if len(streamMessages) < streamReadCount {
			break
		}
	}

	logger.Info(fmt.Sprintf("[SSE Stream Handler] Finished reading history (%d entries) for stream: '%s'. Last ID: %s. Starting live block.", historyProcessed, redisStreamName, lastProcessedID))

	// Read Live Updates (Blocking).
	for {
		// Check context before blocking read.
		select {
		case <-ctx.Done():
			logger.Info(fmt.Sprintf("[SSE Stream Handler] Context done before blocking read for runId %s: %v", runId, ctx.Err()))
			return
		default:
			// Continue to blocking read.
		}

		// logger.Info(fmt.Sprintf("[SSE Stream Handler] Blocking read on stream '%s' from ID: %s", redisStreamName, lastProcessedID))
		cmd := util.RedisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{redisStreamName, lastProcessedID},
			Count:   streamReadCount,
			Block:   blockTimeout,
		})
		results, err := cmd.Result()

		if err != nil {
			// redis.Nil means the block timeout was reached, no new messages.
			if errors.Is(err, redis.Nil) {
				// logger.Debug(fmt.Sprintf("[SSE Stream Handler] Block timeout reached for stream '%s', continuing loop.", redisStreamName))
				continue
			} else if errors.Is(err, context.Canceled) {
				// Client disconnected.
				logger.Info(fmt.Sprintf("[SSE Stream Handler] Context cancelled during blocking read for runId %s: %v", runId, ctx.Err()))
				return
			}

			logger.Error(fmt.Sprintf("[SSE Stream Handler] Error during blocking read for stream '%s': %v", redisStreamName, err))
			time.Sleep(1 * time.Second)
			continue
		}

		// Process new messages if any.
		if len(results) > 0 && len(results[0].Messages) > 0 {
			streamMessages := results[0].Messages
			// logger.Info(fmt.Sprintf("[SSE Stream Handler] Processing %d live messages for stream: '%s'", len(streamMessages), redisStreamName))

			for _, msg := range streamMessages {
				logPayloadStr, ok := msg.Values[logDataField].(string)
				if !ok {
					logger.Warn(fmt.Sprintf("[SSE Stream Handler] Invalid live data format in stream '%s', ID '%s': Missing or non-string field '%s'", redisStreamName, msg.ID, logDataField))
					continue
				}

				// Send the payload.
				if !sendSSEData(w, rc, logPayloadStr, runId, &logger) {
					return
				}

				// Check if this message is the EOF marker.
				var logData redisLogPayload
				if json.Unmarshal([]byte(logPayloadStr), &logData) == nil && logData.Status == eofStatus {
					logger.Info(fmt.Sprintf("[SSE Stream Handler] Live EOF marker found (ID: %s) for runId: %s. Sending done event.", msg.ID, runId))
					doneData := `{"message": "Stream ended."}`
					_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", sseDoneEvent, doneData)
					_ = rc.Flush()
					return
				}
				// Update last ID processed.
				lastProcessedID = msg.ID
			}
		}
	}
}
