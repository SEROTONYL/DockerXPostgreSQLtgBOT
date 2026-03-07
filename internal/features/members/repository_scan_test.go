package members

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

type fakeMemberScanner struct {
	values []interface{}
	err    error
}

func (f fakeMemberScanner) Scan(dest ...interface{}) error {
	if f.err != nil {
		return f.err
	}
	for i := range dest {
		switch d := dest[i].(type) {
		case *int64:
			*d = f.values[i].(int64)
		case *bool:
			*d = f.values[i].(bool)
		case *sql.NullString:
			if f.values[i] == nil {
				*d = sql.NullString{}
				continue
			}
			*d = sql.NullString{String: f.values[i].(string), Valid: true}
		case *string:
			*d = f.values[i].(string)
		case **string:
			if f.values[i] == nil {
				*d = nil
				continue
			}
			v := f.values[i].(string)
			*d = &v
		case **time.Time:
			if f.values[i] == nil {
				*d = nil
				continue
			}
			switch v := f.values[i].(type) {
			case time.Time:
				vv := v
				*d = &vv
			case *time.Time:
				*d = v
			default:
				return errors.New("unsupported time destination value")
			}
		case *time.Time:
			*d = f.values[i].(time.Time)
		default:
			return errors.New("unsupported destination type")
		}
	}
	return nil
}

func TestScanMember_NormalizesNullableTextFieldsToEmptyStrings(t *testing.T) {
	now := time.Now().UTC()
	scanner := fakeMemberScanner{values: []interface{}{
		int64(1), int64(1001), nil, "Ivan", nil,
		nil, false, false, false,
		StatusActive, &now, nil, nil, nil, nil, nil, nil, now, now,
	}}

	var m Member
	if err := scanMember(scanner, &m); err != nil {
		t.Fatalf("scanMember returned error: %v", err)
	}

	if m.Username != "" {
		t.Fatalf("expected empty username for NULL, got %q", m.Username)
	}
	if m.LastName != "" {
		t.Fatalf("expected empty last_name for NULL, got %q", m.LastName)
	}
	if m.FirstName != "Ivan" {
		t.Fatalf("expected first name preserved, got %q", m.FirstName)
	}
	if m.IsBot {
		t.Fatalf("expected is_bot false, got true")
	}
}

func TestScanMember_HandlesNullableFieldsUsedByUsersWithoutRoleAndUsersWithRole(t *testing.T) {
	now := time.Now().UTC()
	role := "captain"
	tag := "TEAM-A"
	scanner := fakeMemberScanner{values: []interface{}{
		int64(2), int64(2002), nil, nil, nil,
		role, true, false, false,
		StatusActive, nil, nil, nil, &now, nil, tag, &now, now, now,
	}}

	var m Member
	if err := scanMember(scanner, &m); err != nil {
		t.Fatalf("scanMember returned error: %v", err)
	}

	if m.Username != "" {
		t.Fatalf("expected empty username for NULL, got %q", m.Username)
	}
	if m.FirstName != "" {
		t.Fatalf("expected empty first_name for NULL, got %q", m.FirstName)
	}
	if m.LastName != "" {
		t.Fatalf("expected empty last_name for NULL, got %q", m.LastName)
	}
	if m.Role == nil || *m.Role != role {
		t.Fatalf("expected role %q, got %#v", role, m.Role)
	}
	if m.Tag == nil || *m.Tag != tag {
		t.Fatalf("expected tag %q, got %#v", tag, m.Tag)
	}
	if m.LastSeenAt == nil {
		t.Fatal("expected last_seen_at to be populated")
	}
}

func TestScanMember_UsersWithRolePath_PreservesRoleAndNullableMetadata(t *testing.T) {
	now := time.Now().UTC()
	role := "moderator"
	tag := "TEAM-B"
	lastKnownName := "Fallback Name"
	scanner := fakeMemberScanner{values: []interface{}{
		int64(3), int64(3003), nil, nil, nil,
		role, true, true, false,
		StatusActive, nil, nil, nil, &now, lastKnownName, tag, &now, now, now,
	}}

	var m Member
	if err := scanMember(scanner, &m); err != nil {
		t.Fatalf("scanMember returned error: %v", err)
	}

	if m.Username != "" || m.FirstName != "" || m.LastName != "" {
		t.Fatalf("expected nullable identity fields normalized to empty strings, got username=%q first_name=%q last_name=%q", m.Username, m.FirstName, m.LastName)
	}
	if m.Role == nil || *m.Role != role {
		t.Fatalf("expected role %q, got %#v", role, m.Role)
	}
	if m.Tag == nil || *m.Tag != tag {
		t.Fatalf("expected tag %q, got %#v", tag, m.Tag)
	}
	if m.LastKnownName == nil || *m.LastKnownName != lastKnownName {
		t.Fatalf("expected last_known_name %q, got %#v", lastKnownName, m.LastKnownName)
	}
	if m.LastSeenAt == nil || !m.LastSeenAt.Equal(now) {
		t.Fatalf("expected last_seen_at %v, got %#v", now, m.LastSeenAt)
	}
	if m.TagUpdatedAt == nil || !m.TagUpdatedAt.Equal(now) {
		t.Fatalf("expected tag_updated_at %v, got %#v", now, m.TagUpdatedAt)
	}
	if !m.IsBot {
		t.Fatalf("expected is_bot true, got false")
	}
}

func TestNullableTextToString(t *testing.T) {
	if got := nullableTextToString(sql.NullString{String: "abc", Valid: true}); got != "abc" {
		t.Fatalf("expected value to pass through, got %q", got)
	}
	if got := nullableTextToString(sql.NullString{}); got != "" {
		t.Fatalf("expected empty string for invalid NULL value, got %q", got)
	}
}
