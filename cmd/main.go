package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/RogueAlmond70/Muzz/explorepb"
	"github.com/RogueAlmond70/Muzz/internal/datastore/postgres"
	redis_cache "github.com/RogueAlmond70/Muzz/internal/datastore/redis"
	"github.com/RogueAlmond70/Muzz/internal/handler"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func main() {

	ctx := context.Background()

	// Set up logger (in production I might have an interface so we're not tightly coupled).
	zlog, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer zlog.Sync()

	redisAddr := getRedisAddr()
	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr, // or use REDIS_URL env var for Docker/cloud
		Password: "",        // set if needed
		DB:       0,         // use default DB
	})
	// Check the connection.
	if err := redisClient.Ping(ctx).Err(); err != nil {
		zlog.Error("failed to connect to redis", zap.Error(err))
		os.Exit(1)
	}

	// Create a postgres instance.
	db, err := postgres.ConnectDBWithRetry(10, 2*time.Second)
	if err != nil {
		zlog.Error("could not connect to DB", zap.Error(err))
		os.Exit(1)
	}

	// Create our postgres client.
	pgClient := postgres.NewPostgres(zlog, db)

	// Create our concrete impementation of the cache interface.
	cache := redis_cache.NewRedisCache(redisClient, redis_cache.ExploreTTL)

	// Create our server to handle requests to our endpoints.
	srv := &handler.Server{
		DB:    pgClient,
		Cache: cache,
	}

	// Set up Prometheus.
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		zlog.Info("Prometheus metrics available at :2112/metrics")
		if err := http.ListenAndServe(":2112", nil); err != nil {
			zlog.Error("Metrics server failed", zap.Error(err))
		}
	}()

	// Set up our grpc server.
	lis, err := net.Listen("tcp", ":9001")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()

	explorepb.RegisterExploreServiceServer(grpcServer, srv)
	log.Println("gRPC server listening on :9001")

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}

func getRedisAddr() string {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost" // default for local runs
	}
	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}
	return fmt.Sprintf("%s:%s", host, port)
}
