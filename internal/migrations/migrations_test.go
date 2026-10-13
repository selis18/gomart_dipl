package migrations

import (
	"strings"
	"testing"
)

func TestInitialSchemaEmbedded(t *testing.T) {
	if !strings.Contains(createTablesSQL, "CREATE TABLE IF NOT EXISTS users") {
		t.Fatal("initial users schema is missing from the binary; check the go:embed directive")
	}
}
