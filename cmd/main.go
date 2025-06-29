package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/RogueAlmond70/Muzz/explorepb"
	"github.com/RogueAlmond70/Muzz/internal/config"
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
	defer func() {
		if err := zlog.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to sync logger: %v\n", err)
		}
	}()

	cfg, err := config.LoadConfig()
	if err != nil {
		zlog.Error("error loading config", zap.Error(err))
		os.Exit(1)
	}

	// Set up redis.
	redisAddr := fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort)

	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: cfg.RedisPassword,
		DB:       0,
	})
	// Check the connection.
	if err := redisClient.Ping(ctx).Err(); err != nil {
		zlog.Error("failed to connect to redis", zap.Error(err))
		os.Exit(1)
	}

	// Create a postgres instance.
	db, err := postgres.ConnectDBWithRetry(*cfg, *zlog)
	if err != nil {
		zlog.Error("could not connect to DB", zap.Error(err))
		os.Exit(1)
	}

	// Create our postgres client.
	pgClient := postgres.NewPostgres(zlog, db, *cfg)

	// Create our concrete impementation of the cache interface.
	cache := redis_cache.NewRedisCache(redisClient, cfg.ExploreTTL)

	// Create our server to handle requests to our endpoints.
	srv := &handler.Server{
		DB:    pgClient,
		Cache: cache,
		Cfg:   cfg,
	}

	// Set up Prometheus.
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		zlog.Info("Prometheus metrics available",
			zap.String("host", cfg.PrometheusPort),
			zap.String("path", "/metrics"),
		)
		if err := http.ListenAndServe(":"+cfg.PrometheusPort, nil); err != nil {
			zlog.Error("Metrics server failed", zap.Error(err))
		}
	}()

	// Set up our grpc server.
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		zlog.Fatal("failed to listen", zap.Error(err))
	}
	grpcServer := grpc.NewServer()

	explorepb.RegisterExploreServiceServer(grpcServer, srv)
	zlog.Info("gRPC server listening", zap.String("port", cfg.GRPCPort))

	if err := grpcServer.Serve(lis); err != nil {
		zlog.Fatal("failed to serve gRPC", zap.Error(err))
	}
}
