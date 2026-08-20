package database

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestParseMigrationUnitsPreservesHistoricalNamesAndOrder(t *testing.T) {
	content := "-- migration: schema.up.sql\nCREATE TABLE base(id int);\n-- migration: 012_feature.up.sql\nALTER TABLE base ADD COLUMN name text;\n"
	units, err := parseMigrationUnits("schema.up.sql", content)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 2 || units[0].Name != "schema.up.sql" || units[1].Name != "012_feature.up.sql" {
		t.Fatalf("unexpected units: %#v", units)
	}
}

func TestParseMigrationUnitsRejectsDuplicateNames(t *testing.T) {
	content := "-- migration: schema.up.sql\nSELECT 1;\n-- migration: schema.up.sql\nSELECT 2;\n"
	if _, err := parseMigrationUnits("schema.up.sql", content); err == nil {
		t.Fatal("expected duplicate section error")
	}
}

func TestRepositoryContainsOnlyCanonicalSchemaPair(t *testing.T) {
	migrationsDir := filepath.Join("..", "..", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	var sqlFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
			sqlFiles = append(sqlFiles, entry.Name())
		}
	}
	sort.Strings(sqlFiles)
	want := []string{"schema.down.sql", "schema.up.sql"}
	if !reflect.DeepEqual(sqlFiles, want) {
		t.Fatalf("migration directory must contain only the canonical schema pair: got %v", sqlFiles)
	}

	content, err := os.ReadFile(filepath.Join(migrationsDir, "schema.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	units, err := parseMigrationUnits("schema.up.sql", string(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 11 {
		t.Fatalf("expected 11 ordered logical migrations, got %d", len(units))
	}
	if units[0].Name != "schema.up.sql" || units[len(units)-1].Name != "020_celebration_automation_config_repair.up.sql" {
		t.Fatalf("unexpected migration boundaries: first=%q last=%q", units[0].Name, units[len(units)-1].Name)
	}
}
