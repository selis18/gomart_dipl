package config

import "github.com/caarlos0/env/v11"

type Config struct {
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	SecretKey string `env:"SECRET_KEY,required,notEmpty"`
}

func ParseConfig() (Config, error) {
	var cfg Config
	err := env.Parse(&cfg)
	return cfg, err
}
