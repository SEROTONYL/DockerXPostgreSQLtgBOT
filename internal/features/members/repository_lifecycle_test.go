package members

import (
	"strings"
	"testing"
)

func TestMemberQueries_UseActiveStatusFilter(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "list active members", query: listActiveMembersQuery(), want: "WHERE status = $1"},
		{name: "get by username", query: getByUsernameQuery(), want: "AND status = $2"},
		{name: "users without role", query: usersWithoutRoleQuery(), want: "status = $1"},
		{name: "users with role", query: usersWithRoleQuery(), want: "status = $1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.query, tc.want) {
				t.Fatalf("query does not contain %q: %s", tc.want, tc.query)
			}
		})
	}
}

func TestUpsertActiveMemberQuery_ClearsLeftLifecycleFields(t *testing.T) {
	q := upsertActiveMemberQuery()
	checks := []string{
		"status = $4",
		"left_at = NULL",
		"delete_after = NULL",
	}
	for _, c := range checks {
		if !strings.Contains(q, c) {
			t.Fatalf("upsert query missing %q: %s", c, q)
		}
	}
}

func TestPurgeDeleteQueries_RemoveDomainDataBeforeMembers(t *testing.T) {
	queries := purgeDeleteQueries()

	find := func(part string) int {
		for i, q := range queries {
			if strings.Contains(q, part) {
				return i
			}
		}
		return -1
	}

	idxTransactions := find("DELETE FROM transactions")
	idxBalances := find("DELETE FROM balances")
	idxMembers := find("DELETE FROM members")

	if idxTransactions < 0 || idxBalances < 0 || idxMembers < 0 {
		t.Fatalf("purge queries must include transactions, balances and members, got: %#v", queries)
	}
	if idxTransactions > idxMembers {
		t.Fatalf("transactions should be deleted before members: tx=%d members=%d", idxTransactions, idxMembers)
	}
	if idxBalances > idxMembers {
		t.Fatalf("balances should be deleted before members: balances=%d members=%d", idxBalances, idxMembers)
	}
}

func TestEnsureMemberSeenQuery_UsesThrottleCondition(t *testing.T) {
	q := ensureMemberSeenQuery()
	checks := []string{
		"UPDATE members",
		"WHERE user_id = $1",
		"last_seen_at < $4 - INTERVAL '5 minutes'",
	}
	for _, c := range checks {
		if !strings.Contains(q, c) {
			t.Fatalf("ensure seen query missing %q: %s", c, q)
		}
	}
}

func TestEnsureActiveMemberSeenQuery_UsesThrottleCondition(t *testing.T) {
	q := ensureActiveMemberSeenQuery()
	checks := []string{
		"INSERT INTO members",
		"ON CONFLICT (user_id) DO UPDATE",
		"members.last_seen_at < $5 - INTERVAL '5 minutes'",
	}
	for _, c := range checks {
		if !strings.Contains(q, c) {
			t.Fatalf("ensure active seen query missing %q: %s", c, q)
		}
	}
}

func TestCountMembersByStatusQuery_UsesBothStatuses(t *testing.T) {
	q := countMembersByStatusQuery()
	checks := []string{
		"COUNT(*) FILTER (WHERE status = $1)",
		"COUNT(*) FILTER (WHERE status = $2)",
		"FROM members",
	}
	for _, c := range checks {
		if !strings.Contains(q, c) {
			t.Fatalf("count query missing %q: %s", c, q)
		}
	}
}

func TestCountPendingPurgeQuery_UsesLeftAndDeleteAfter(t *testing.T) {
	q := countPendingPurgeQuery()
	checks := []string{
		"FROM members",
		"status = $1",
		"delete_after <= $2",
	}
	for _, c := range checks {
		if !strings.Contains(q, c) {
			t.Fatalf("pending purge query missing %q: %s", c, q)
		}
	}
}

func TestTouchLastSeenQuery_UsesThrottleCondition(t *testing.T) {
	q := touchLastSeenQuery()
	checks := []string{
		"UPDATE members",
		"last_seen_at = $2",
		"user_id = $1",
		"last_seen_at < $2 - INTERVAL '5 minutes'",
	}
	for _, c := range checks {
		if !strings.Contains(q, c) {
			t.Fatalf("touch query missing %q: %s", c, q)
		}
	}
}
