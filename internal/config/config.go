package config

import (
	"github.com/caarlos0/env"
	"go.uber.org/zap"
)

type Config struct {
	LogLevel  string `env:"LOG_LEVEL"`
	SecretKey string `env:"SECRET_KEY"`
}

var secretKey string
var logLevel string

func ParseConfig() {
	cfg := Config{}
	err := env.Parse(&cfg)
	if err != nil {
		zap.NamedError("Config parser error: ", err)
	}

	if cfg.SecretKey != "" {
		secretKey = cfg.SecretKey
	}

	if cfg.LogLevel != "" {
		logLevel = cfg.LogLevel
	}
}

func GetSecretKey() string {
	return secretKey
}

func GetLogLevel() string {
	return logLevel
}
