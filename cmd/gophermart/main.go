package main

import (
	"log"
	"os"

	"github.com/selis18/gomart_dipl/internal/config"
	"github.com/selis18/gomart_dipl/internal/logger"
)

func main() {
	cfg, err := config.ParseConfig(os.Args[1:])
	if err != nil {
		log.Fatalf("parse configuration: %v", err)
	}
	level := cfg.LogLevel
	if err := logger.Initialize(level); err != nil {
		log.Fatal(err)
	}
	if err := InitServer(cfg); err != nil {
		log.Fatal(err)
	}
}
