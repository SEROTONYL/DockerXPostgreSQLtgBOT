package migrations

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIsUpMigrationFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
		wantVer  int
		wantOK   bool
	}{
		{name: "valid up", filename: "0001_init.sql", wantVer: 1, wantOK: true},
		{name: "valid up two digits", filename: "0010_x.sql", wantVer: 10, wantOK: true},
		{name: "reject six-digit down", filename: "000001_init.down.sql", wantOK: false},
		{name: "reject four-digit down", filename: "0001_init.down.sql", wantOK: false},
		{name: "reject non matching", filename: "readme.sql", wantOK: false},
		{name: "reject path", filename: "migrations/0001_init.sql", wantOK: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotVer, gotOK := isUpMigrationFile(tc.filename)
			if gotOK != tc.wantOK {
				t.Fatalf("isUpMigrationFile(%q) ok = %v, want %v", tc.filename, gotOK, tc.wantOK)
			}
			if gotVer != tc.wantVer {
				t.Fatalf("isUpMigrationFile(%q) version = %d, want %d", tc.filename, gotVer, tc.wantVer)
			}
		})
	}
}

func TestFilterPending(t *testing.T) {
	t.Parallel()

	files := []migrationFile{
		{version: 1, filename: "0001_init.sql"},
		{version: 2, filename: "0002_economy.sql"},
		{version: 3, filename: "0003_streaks.sql"},
	}
	applied := map[int]struct{}{1: {}, 3: {}}

	pending := filterPending(files, applied)
	if len(pending) != 1 {
		t.Fatalf("pending count = %d, want 1", len(pending))
	}
	if pending[0].version != 2 {
		t.Fatalf("pending version = %d, want 2", pending[0].version)
	}
}

func TestCollectMigrationFilesSortsByVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	create := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1;"), 0o600); err != nil {
			t.Fatalf("write migration file: %v", err)
		}
	}

	create("0003_c.sql")
	create("0001_a.sql")
	create("0002_b.sql")
	create("0002_b.down.sql")
	create("000001_legacy.down.sql")
	create("README.sql")

	files, err := collectMigrationFiles(dir + "/*.sql")
	if err != nil {
		t.Fatalf("collectMigrationFiles() error: %v", err)
	}

	want := []int{1, 2, 3}
	if len(files) != len(want) {
		t.Fatalf("files count = %d, want %d", len(files), len(want))
	}
	for i, v := range want {
		if files[i].version != v {
			t.Fatalf("files[%d].version = %d, want %d", i, files[i].version, v)
		}
	}
}

func TestBuildMigrationFilesIgnoresDownAndInvalid(t *testing.T) {
	t.Parallel()

	filenames := []string{
		"0003_streaks.sql",
		"0001_init.sql",
		"0001_init.down.sql",
		"000001_init.down.sql",
		"readme.sql",
		"0010_feature.sql",
	}

	files, err := buildMigrationFiles(filenames)
	if err != nil {
		t.Fatalf("buildMigrationFiles() error: %v", err)
	}

	got := make([]string, 0, len(files))
	for _, f := range files {
		got = append(got, f.filename)
	}

	want := []string{"0001_init.sql", "0003_streaks.sql", "0010_feature.sql"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMigrationFiles() filenames = %v, want %v", got, want)
	}
}

func TestBuildMigrationFilesDuplicateVersion(t *testing.T) {
	t.Parallel()

	_, err := buildMigrationFiles([]string{"0001_a.sql", "0001_b.sql"})
	if err == nil {
		t.Fatal("buildMigrationFiles() expected duplicate version error")
	}
}
