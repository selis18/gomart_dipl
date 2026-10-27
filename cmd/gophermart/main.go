package main

import (
	"log"

	"github.com/selis18/gomart_dipl/internal/config"
	"github.com/selis18/gomart_dipl/internal/logger"
)

func main() {
	cfg, err := config.ParseConfig()
	if err != nil {
		log.Fatalf("parse configuration: %v", err)
	}
	level := cfg.LogLevel
	if err := logger.Initialize(level); err != nil {
		log.Fatal(err)
	}
	if err := InitServer(cfg.SecretKey); err != nil {
		log.Fatal(err)
	}
}
