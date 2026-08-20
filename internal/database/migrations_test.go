package database

import "testing"

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
