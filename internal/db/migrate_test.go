package db

import (
	"testing"
)

func TestInitialMigrationEmbedded(t *testing.T) {
	if initialMigration == "" {
		t.Fatal("initial migration SQL must not be empty")
	}
}
