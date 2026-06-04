package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMigrationVersion_Extracts(t *testing.T) {
	assert.Equal(t, "001", migrationVersion("001_create_workflow_tables.sql"))
	assert.Equal(t, "002", migrationVersion("002_create_eval_tables.sql"))
}

func TestMigrationVersion_NoUnderscore(t *testing.T) {
	assert.Equal(t, "003", migrationVersion("003.sql"))
}

func TestMigrationVersion_DeepPath(t *testing.T) {
	assert.Equal(t, "004", migrationVersion("migrations/004_add_index.sql"))
}

func TestMigrationVersion_Empty(t *testing.T) {
	assert.Equal(t, ".", migrationVersion(""))
}

func TestMigrationVersion_NoLeadingDigits(t *testing.T) {
	assert.Equal(t, "create", migrationVersion("create_table.sql"))
}
