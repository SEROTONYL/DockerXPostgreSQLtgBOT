package members

import (
	"strings"
	"testing"
)

func TestPurgeSelectionQuery_HasExpectedFilters(t *testing.T) {
	q := purgeSelectionQuery()

	checks := []string{
		"FROM members",
		"status = $1",
		"delete_after IS NOT NULL",
		"delete_after <= $2",
		"LIMIT $3",
	}
	for _, c := range checks {
		if !strings.Contains(q, c) {
			t.Fatalf("query missing %q: %s", c, q)
		}
	}
}

func TestPurgeDeleteQueries_EndsWithMembersDelete(t *testing.T) {
	queries := purgeDeleteQueries()
	if len(queries) == 0 {
		t.Fatal("expected non-empty queries")
	}
	last := queries[len(queries)-1]
	if !strings.Contains(last, "DELETE FROM members") {
		t.Fatalf("last query should delete members, got: %s", last)
	}
}
