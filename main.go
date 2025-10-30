package main

import (
	"context"
	"errors"
	"evolve/controller"
	"evolve/modules/sse"
	"evolve/routes"
	"evolve/util"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/cors"
)

var (
	PORT         string
	FRONTEND_URL string
)

func main() {

	logger, err := util.InitLogger(os.Getenv("ENV"))
	util.SharedLogger = logger
	PORT = fmt.Sprintf(":%s", os.Getenv("HTTP_PORT"))
	if PORT == ":" {
		PORT = ":5002"
	}
	FRONTEND_URL = os.Getenv("FRONTEND_URL")
	if FRONTEND_URL == "" {
		FRONTEND_URL = "http://localhost:3000"
	}

	redisClient, err := sse.GetRedisClient(*logger)

	if err != nil {
		logger.Error(fmt.Sprintf("Failed to initialize Redis client: %v. Exiting.", err), err)
		os.Exit(1)
	}
	logger.Info("Redis client initialized successfully.")

	// Use context for cancellation signal propagation.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Register HTTP Routes
	mux := http.NewServeMux()

	mux.HandleFunc(routes.TEST, controller.Test)
	mux.HandleFunc(routes.EA, controller.CreateEA)
	mux.HandleFunc(routes.GP, controller.CreateGP)
	mux.HandleFunc(routes.ML, controller.CreateML)
	mux.HandleFunc(routes.PSO, controller.CreatePSO)
	mux.HandleFunc(routes.RUNS, controller.UserRuns)
	mux.HandleFunc(routes.SHARE_RUN, controller.ShareRun)
	mux.HandleFunc(routes.RUN, controller.UserRun)

	sseHandler := sse.GetSSEHandler(*logger, redisClient)
	mux.HandleFunc(routes.LOGS, sseHandler)
	logger.Info(fmt.Sprintf("SSE endpoint registered at %s using Redis Pub/Sub", routes.LOGS))

	logger.Info(fmt.Sprintf("Test http server on http://localhost%s/api/test", PORT))

	// CORS Configuration.
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   []string{FRONTEND_URL},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*", "X-RUN-ID"},
		ExposedHeaders:   []string{},
		AllowCredentials: true,
	}).Handler(mux)

	// HTTP Server Setup and Start.
	server := &http.Server{
		Addr:         PORT,
		Handler:      corsHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  0,
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
	}

	// Start server in a goroutine so
	// it doesn't block the graceful shutdown handling.
	go func() {
		logger.Info(fmt.Sprintf("HTTP server starting on %s (Allowed Frontend Origin: %s)", server.Addr, FRONTEND_URL))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(fmt.Sprintf("HTTP server ListenAndServe error: %v", err), err)
			stop()
		}
	}()

	// Wait for Shutdown Signal.
	<-ctx.Done()

	logger.Info("Shutdown signal received. Starting graceful shutdown...")

	// Create a context with a timeout for the shutdown process.
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()

	// Attempt to gracefully shut down the HTTP server.
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error(fmt.Sprintf("HTTP server graceful shutdown failed: %v", err), err)
	} else {
		logger.Info("HTTP server shutdown complete.")
	}

	// Close Redis Client.
	logger.Info("Shutting down Redis client...")
	if redisErr := redisClient.Close(); redisErr != nil {
		logger.Error(fmt.Sprintf("Redis client shutdown error: %v", redisErr), redisErr)
	} else {
		logger.Info("Redis client shutdown complete.")
	}

	logger.Info("Server exiting.")
}
