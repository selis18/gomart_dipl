package config

import "testing"

func TestParseConfigDoesNotRetainState(t *testing.T) {
	t.Setenv("SECRET_KEY", "first")
	t.Setenv("LOG_LEVEL", "debug")
	first, err := ParseConfig()
	if err != nil || first.SecretKey != "first" || first.LogLevel != "debug" {
		t.Fatalf("config=%+v err=%v", first, err)
	}
	t.Setenv("SECRET_KEY", "second")
	t.Setenv("LOG_LEVEL", "error")
	second, err := ParseConfig()
	if err != nil || second.SecretKey != "second" || second.LogLevel != "error" || first.SecretKey != "first" {
		t.Fatal("configuration state was shared")
	}
	t.Setenv("SECRET_KEY", "")
	if _, err := ParseConfig(); err == nil {
		t.Fatal("empty secret accepted")
	}
}
