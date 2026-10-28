package config

import (
	"flag"
	"os"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Address        string
	DatabaseURI    string
	AccrualAddress string
	LogLevel       string `env:"LOG_LEVEL" envDefault:"info"`
	SecretKey      string `env:"SECRET_KEY,required,notEmpty"`
}

func ParseConfig(args []string) (Config, error) {
	cfg := Config{Address: "localhost:8080"}
	if err := env.Parse(&cfg); err != nil {
		return cfg, err
	}
	if address, ok := os.LookupEnv("RUN_ADDRESS"); ok {
		cfg.Address = address
	}
	if databaseURI, ok := os.LookupEnv("DATABASE_URI"); ok {
		cfg.DatabaseURI = databaseURI
	}
	if accrualAddress, ok := os.LookupEnv("ACCRUAL_SYSTEM_ADDRESS"); ok {
		cfg.AccrualAddress = accrualAddress
	}

	flags := flag.NewFlagSet("gophermart", flag.ContinueOnError)
	flags.StringVar(&cfg.Address, "a", cfg.Address, "HTTP listen address")
	flags.StringVar(&cfg.DatabaseURI, "d", cfg.DatabaseURI, "PostgreSQL URI")
	flags.StringVar(&cfg.AccrualAddress, "r", cfg.AccrualAddress, "accrual service address")
	err := flags.Parse(args)
	return cfg, err
}
