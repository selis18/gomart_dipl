package main

import "testing"

func TestRuntimeConfig(t *testing.T) {
	t.Setenv("RUN_ADDRESS", "localhost:9090")
	t.Setenv("DATABASE_URI", "postgres://environment")
	t.Setenv("ACCRUAL_SYSTEM_ADDRESS", "http://localhost:9091")
	cfg, err := parseRuntimeConfig(nil)
	if err != nil || cfg.address != "localhost:9090" || cfg.databaseURI != "postgres://environment" || cfg.accrualAddress != "http://localhost:9091" {
		t.Fatalf("config=%+v error=%v", cfg, err)
	}
	cfg, err = parseRuntimeConfig([]string{"-a", "localhost:8080", "-d", "postgres://flag", "-r", "http://localhost:8081"})
	if err != nil || cfg.address != "localhost:8080" || cfg.databaseURI != "postgres://flag" || cfg.accrualAddress != "http://localhost:8081" {
		t.Fatalf("config=%+v error=%v", cfg, err)
	}
	if _, err := parseRuntimeConfig([]string{"-unknown"}); err == nil {
		t.Fatal("unknown flag accepted")
	}
}
