package security

import (
	"crypto/subtle"
	"fmt"
	"strings"
)

// KeyStore holds the static API-key -> Principal mapping loaded once at
// startup from configuration. It never logs or exposes key material --
// only Principal.Name (never the key) is meant to leave this package.
type KeyStore struct {
	// keyed by the raw API key so Lookup is a direct map access before the
	// constant-time comparison step; see Lookup for why a map lookup here
	// doesn't reintroduce a timing side-channel worth worrying about at
	// this project's scale (documented there).
	byKey map[string]Principal
}

// ParseAPIKeys parses the ATLAS_API_KEYS environment variable format:
//
//	name:key:role,name:key:role,...
//
// e.g. "alice:alice-key-123:OPERATOR,bob:bob-key-456:APPROVER". Chosen over
// a structured format (JSON/YAML) because it fits directly into a single
// docker-compose environment entry with no new parsing dependency, matching
// this project's existing ATLAS_*_SECONDS-style single-env-var conventions.
//
// An empty raw string is a valid configuration (zero keys -- every request
// is then treated as unauthenticated, which is the safe default). Any
// malformed entry, duplicate key, or duplicate principal name is a hard
// error: security configuration must fail loudly at startup rather than
// silently accept an ambiguous or broken mapping.
func ParseAPIKeys(raw string) (*KeyStore, error) {
	ks := &KeyStore{byKey: make(map[string]Principal)}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ks, nil
	}

	seenNames := make(map[string]bool)

	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.Split(entry, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("security: malformed ATLAS_API_KEYS entry %q: expected name:key:role", entry)
		}

		name := strings.TrimSpace(parts[0])
		key := strings.TrimSpace(parts[1])
		role := Role(strings.TrimSpace(parts[2]))

		if name == "" || key == "" || role == "" {
			return nil, fmt.Errorf("security: malformed ATLAS_API_KEYS entry %q: name, key, and role must all be non-empty", entry)
		}
		if !IsValidRole(role) {
			return nil, fmt.Errorf("security: ATLAS_API_KEYS entry %q: unrecognized role %q", entry, role)
		}
		if _, exists := ks.byKey[key]; exists {
			return nil, fmt.Errorf("security: ATLAS_API_KEYS contains a duplicate key value (principal %q)", name)
		}
		if seenNames[name] {
			return nil, fmt.Errorf("security: ATLAS_API_KEYS contains a duplicate principal name %q", name)
		}

		seenNames[name] = true
		ks.byKey[key] = Principal{Name: name, Role: role}
	}

	return ks, nil
}

// Lookup resolves an API key to its Principal. Compares the presented key
// against every configured key using subtle.ConstantTimeCompare rather than
// a map lookup on the presented value, so a caller probing for a valid key
// can't learn anything from comparison timing -- every call does the same
// amount of work regardless of whether, or where, a match exists. This
// project's threat model (a local/demo deployment, per the M2.9
// investigation) does not warrant more than this.
func (ks *KeyStore) Lookup(key string) (Principal, bool) {
	if key == "" {
		return Principal{}, false
	}
	presented := []byte(key)
	var match Principal
	found := false
	for stored, principal := range ks.byKey {
		if subtle.ConstantTimeCompare(presented, []byte(stored)) == 1 {
			match = principal
			found = true
		}
	}
	return match, found
}
