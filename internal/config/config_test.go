package config

import (
	"os"
	"testing"
)

func TestParseConfigDoesNotRetainState(t *testing.T) {
	t.Setenv("SECRET_KEY", "first")
	t.Setenv("LOG_LEVEL", "debug")
	first, err := ParseConfig(nil)
	if err != nil || first.SecretKey != "first" || first.LogLevel != "debug" {
		t.Fatalf("config=%+v err=%v", first, err)
	}
	t.Setenv("SECRET_KEY", "second")
	t.Setenv("LOG_LEVEL", "error")
	second, err := ParseConfig(nil)
	if err != nil || second.SecretKey != "second" || second.LogLevel != "error" || first.SecretKey != "first" {
		t.Fatal("configuration state was shared")
	}
	t.Setenv("SECRET_KEY", "")
	if _, err := ParseConfig(nil); err == nil {
		t.Fatal("empty secret accepted")
	}
}

func TestRuntimeConfig(t *testing.T) {
	t.Setenv("SECRET_KEY", "test-secret")
	t.Setenv("RUN_ADDRESS", "localhost:9090")
	t.Setenv("DATABASE_URI", "postgres://environment")
	t.Setenv("ACCRUAL_SYSTEM_ADDRESS", "http://localhost:9091")
	cfg, err := ParseConfig(nil)
	if err != nil || cfg.Address != "localhost:9090" || cfg.DatabaseURI != "postgres://environment" || cfg.AccrualAddress != "http://localhost:9091" {
		t.Fatalf("config=%+v error=%v", cfg, err)
	}
	cfg, err = ParseConfig([]string{"-a", "localhost:8080", "-d", "postgres://flag", "-r", "http://localhost:8081"})
	if err != nil || cfg.Address != "localhost:8080" || cfg.DatabaseURI != "postgres://flag" || cfg.AccrualAddress != "http://localhost:8081" {
		t.Fatalf("config=%+v error=%v", cfg, err)
	}
	if _, err := ParseConfig([]string{"-unknown"}); err == nil {
		t.Fatal("unknown flag accepted")
	}
}

func TestRunAddressAbsentAndEmpty(t *testing.T) {
	t.Setenv("SECRET_KEY", "test-secret")
	t.Setenv("RUN_ADDRESS", "")
	cfg, err := ParseConfig(nil)
	if err != nil || cfg.Address != "" {
		t.Fatalf("empty address: config=%+v err=%v", cfg, err)
	}
	if err := os.Unsetenv("RUN_ADDRESS"); err != nil {
		t.Fatal(err)
	}
	cfg, err = ParseConfig(nil)
	if err != nil || cfg.Address != "localhost:8080" {
		t.Fatalf("absent address: config=%+v err=%v", cfg, err)
	}
}
