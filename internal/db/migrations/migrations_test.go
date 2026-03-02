package migrations

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
		want     int
		wantErr  bool
	}{
		{name: "valid", filename: "0001_init.sql", want: 1},
		{name: "valid with long name", filename: "0012_add_admin_balance_deltas.sql", want: 12},
		{name: "no underscore", filename: "0001.sql", wantErr: true},
		{name: "invalid prefix", filename: "init.sql", wantErr: true},
		{name: "zero version", filename: "0000_init.sql", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseVersion(tc.filename)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseVersion(%q) expected error", tc.filename)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseVersion(%q) unexpected error: %v", tc.filename, err)
			}
			if got != tc.want {
				t.Fatalf("parseVersion(%q) = %d, want %d", tc.filename, got, tc.want)
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
