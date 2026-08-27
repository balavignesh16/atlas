package security

import (
	"strings"
	"testing"
)

func TestParseAPIKeys_Empty(t *testing.T) {
	ks, err := ParseAPIKeys("")
	if err != nil {
		t.Fatalf("unexpected error for empty config: %v", err)
	}
	if _, ok := ks.Lookup("anything"); ok {
		t.Fatal("expected an empty keystore to resolve nothing")
	}
}

func TestParseAPIKeys_Valid(t *testing.T) {
	ks, err := ParseAPIKeys("alice:alice-key:OPERATOR,bob:bob-key:APPROVER")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p, ok := ks.Lookup("alice-key")
	if !ok || p.Name != "alice" || p.Role != RoleOperator {
		t.Fatalf("expected alice/OPERATOR, got %+v ok=%v", p, ok)
	}

	p, ok = ks.Lookup("bob-key")
	if !ok || p.Name != "bob" || p.Role != RoleApprover {
		t.Fatalf("expected bob/APPROVER, got %+v ok=%v", p, ok)
	}
}

func TestParseAPIKeys_WhitespaceTolerant(t *testing.T) {
	ks, err := ParseAPIKeys(" alice : alice-key : OPERATOR , bob:bob-key:APPROVER ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := ks.Lookup("alice-key"); !ok {
		t.Fatal("expected whitespace around fields to be trimmed")
	}
}

func TestParseAPIKeys_MalformedEntry(t *testing.T) {
	cases := []string{
		"alice:alice-key",                // missing role
		"alice:alice-key:OPERATOR:extra", // too many parts
		"::OPERATOR",                     // empty name and key
		"alice::OPERATOR",                // empty key
		"alice:alice-key:",               // empty role
		"alice:alice-key:NOT_A_ROLE",     // unrecognized role
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseAPIKeys(raw); err == nil {
				t.Fatalf("expected an error for malformed entry %q", raw)
			}
		})
	}
}

func TestParseAPIKeys_DuplicateKey(t *testing.T) {
	_, err := ParseAPIKeys("alice:shared-key:OPERATOR,bob:shared-key:APPROVER")
	if err == nil {
		t.Fatal("expected an error for two principals sharing one key")
	}
	if !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("expected error to mention duplicate key, got: %v", err)
	}
}

func TestParseAPIKeys_DuplicateName(t *testing.T) {
	_, err := ParseAPIKeys("alice:key-one:OPERATOR,alice:key-two:APPROVER")
	if err == nil {
		t.Fatal("expected an error for one name mapped to two different keys/roles")
	}
	if !strings.Contains(err.Error(), "duplicate principal name") {
		t.Fatalf("expected error to mention duplicate principal name, got: %v", err)
	}
}

func TestKeyStore_Lookup_UnknownKeyReturnsFalse(t *testing.T) {
	ks, _ := ParseAPIKeys("alice:alice-key:OPERATOR")
	if _, ok := ks.Lookup("not-a-real-key"); ok {
		t.Fatal("expected an unknown key to resolve to nothing")
	}
}

func TestKeyStore_Lookup_EmptyKeyReturnsFalse(t *testing.T) {
	ks, _ := ParseAPIKeys("alice:alice-key:OPERATOR")
	if _, ok := ks.Lookup(""); ok {
		t.Fatal("expected an empty presented key to never match")
	}
}

func TestKeyStore_Lookup_Deterministic(t *testing.T) {
	ks, _ := ParseAPIKeys("alice:alice-key:OPERATOR,bob:bob-key:APPROVER,carol:carol-key:EXECUTOR")
	for i := 0; i < 20; i++ {
		p, ok := ks.Lookup("bob-key")
		if !ok || p.Name != "bob" {
			t.Fatalf("iteration %d: expected stable bob/APPROVER resolution, got %+v ok=%v", i, p, ok)
		}
	}
}
