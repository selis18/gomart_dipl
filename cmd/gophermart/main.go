package main

import (
	"log"

	"github.com/selis18/gomart_dipl/internal/config"
	"github.com/selis18/gomart_dipl/internal/logger"
)

func main() {
	config.ParseConfig()
	if config.GetSecretKey() == "" {
		log.Fatal("SECRET_KEY is empty")
	}
	level := config.GetLogLevel()
	if level == "" {
		level = "info"
	}
	if err := logger.Initialize(level); err != nil {
		log.Fatal(err)
	}
	if err := InitServer(); err != nil {
		log.Fatal(err)
	}
}
