package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	RedisHost        string
	RedisPort        string
	RedisPassword    string
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresRetry    int
	PostgresDelay    time.Duration
	ExploreTTL       time.Duration
	PrometheusPort   string
	GRPCPort         string
	PageSize         int
}

func LoadConfig() (*Config, error) {
	exploreTTL, err := time.ParseDuration(getEnv("EXPLORE_TTL", "30s"))
	if err != nil {
		return nil, fmt.Errorf("error parsing duration for TTL environment variable: %w", err)
	}
	postgresRetry, err := strconv.Atoi(getEnv("POSTGRES_RETRY", "10"))
	if err != nil {
		return nil, fmt.Errorf("error parsing POSTGRES_RETRY: %w", err)
	}
	postgresDelay, err := time.ParseDuration(getEnv("POSTGRES_DELAY", "2s"))
	if err != nil {
		return nil, fmt.Errorf("error parsing duration for Postgres Delay environment variable: %w", err)
	}
	pageSize, err := strconv.Atoi(getEnv("PAGE_SIZE", "25"))
	if err != nil {
		return nil, fmt.Errorf("error parsing PAGE_SIZE: %w", err)
	}

	return &Config{
		RedisHost:        getEnv("REDIS_HOST", "localhost"),
		RedisPort:        getEnv("REDIS_PORT", "6379"),
		RedisPassword:    getEnv("REDIS_PASSWORD", ""),
		PostgresHost:     getEnv("POSTGRES_HOST", "localhost"),
		PostgresPort:     getEnv("POSTGRES_PORT", "5432"),
		PostgresUser:     getEnv("POSTGRES_USER", "postgres"),
		PostgresPassword: getEnv("POSTGRES_PASSWORD", "password"),
		PostgresDB:       getEnv("POSTGRES_DB", "myappdb"),
		PostgresRetry:    postgresRetry,
		PostgresDelay:    postgresDelay,
		ExploreTTL:       exploreTTL,
		PrometheusPort:   getEnv("PROMETHEUS_PORT", "2112"),
		GRPCPort:         getEnv("GRPC_PORT", "9001"),
		PageSize:         pageSize,
	}, nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}
