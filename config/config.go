package config

import (
	"fmt"
	"os"
)

// Config содержит настройки приложения.
type Config struct {
	DBHost     string
	DBUser     string
	DBPassword string
	DBName     string
	HTTPPort   string
}

// Load читает все настройки из окружения и завершает программу,
// если обнаружит отсутствие обязательной переменной.
func Load() *Config {
	return &Config{
		DBHost:     getEnvOrFatal("DB_HOST"),
		DBUser:     getEnvOrFatal("DB_USER"),
		DBPassword: getEnvOrFatal("DB_PASSWORD"),
		DBName:     getEnvOrFatal("DB_NAME"),
		HTTPPort:   getEnvOrDefault("HTTP_PORT", "8000"),
	}
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvOrFatal(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	fmt.Fprintf(os.Stderr, "Missing required environment variable: %s\n", key)
	os.Exit(1)
	return "" // не будет достигнуто
}
