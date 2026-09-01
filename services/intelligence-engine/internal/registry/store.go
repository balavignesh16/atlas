package registry

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver: required because this repo builds with CGO_ENABLED=0
)

const schema = `
CREATE TABLE IF NOT EXISTS services (
	name                  TEXT PRIMARY KEY,
	display_name          TEXT NOT NULL,
	provenance            TEXT NOT NULL,
	status                TEXT NOT NULL,
	first_observed_at     TEXT NOT NULL,
	last_observed_at      TEXT NOT NULL,
	last_telemetry_at     TEXT,
	authority_observed_at TEXT NOT NULL DEFAULT '',
	created_at            TEXT NOT NULL,
	updated_at            TEXT NOT NULL
);
`

// migrateAuthorityColumn adds authority_observed_at to a services table
// created by Phase 7B, before this column existed. SQLite has no
// "ADD COLUMN IF NOT EXISTS" in the version modernc.org/sqlite vendors, so
// the duplicate-column error (the only failure mode once the table itself
// exists) is deliberately swallowed. Backfills every pre-existing row's
// authority_observed_at from its own last_observed_at, which is exactly
// correct for that data: Phase 7B only ever had one evidence source, so
// "the last time anything was observed" and "the last time the
// authoritative source was confirmed" were always the same timestamp.
func (s *Store) migrateAuthorityColumn() error {
	_, err := s.db.Exec(`ALTER TABLE services ADD COLUMN authority_observed_at TEXT NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return fmt.Errorf("registry: migrate authority_observed_at: %w", err)
	}
	if _, err := s.db.Exec(`UPDATE services SET authority_observed_at = last_observed_at WHERE authority_observed_at = ''`); err != nil {
		return fmt.Errorf("registry: backfill authority_observed_at: %w", err)
	}
	return nil
}

// Store is the canonical service registry's persistence layer: one SQLite
// table, no ORM, no migrations framework -- the smallest thing that
// survives a process restart with deterministic behavior. Timestamps are
// stored as RFC3339Nano text (SQLite has no native datetime type; text
// sorts and parses unambiguously, unlike relying on unixepoch integers
// mixed with human inspection).
type Store struct {
	db *sql.DB
}

// NewStore opens (creating if necessary) a SQLite database at dbPath and
// ensures the schema exists. Pass ":memory:" for tests. A single
// connection is deliberate: SQLite serializes writers regardless, this
// process is the only writer, and one connection keeps behavior
// deterministic instead of racing against SQLite's own locking under a
// pooled multi-connection setup this app's write volume doesn't need.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("registry: open %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("registry: migrate: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrateAuthorityColumn(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Observe is a thin, backward-compatible convenience wrapper around
// Record for the one source that actually exists today: real telemetry.
// internal/ingestion calls this exact signature, unchanged since Phase 7B.
func (s *Store) Observe(name string, at time.Time) error {
	return s.Record(Evidence{ServiceName: name, Source: ProvenanceObservedTelemetry, ObservedAt: at})
}

// Record is the one way ANY source (today: OBSERVED_TELEMETRY; later:
// DOCKER/KUBERNETES/DECLARED/CONFIG/INFERRED, once implemented) reports
// evidence that a service exists. It is the general mechanism Observe is
// built on top of.
//
// Semantics, deliberately split into two independent, order-independent
// reductions over however many times Record is called for the same name:
//
//  1. Existence/recency (LastObservedAt, Status): ALWAYS advances to
//     max(current, evidence.ObservedAt) and reactivates to ACTIVE,
//     regardless of the evidence's source or precedence. Whether a service
//     still exists does not depend on which source is trusted for its
//     identity -- even weak evidence that it's still around should not be
//     discarded because a stronger source described it once, long ago.
//     FirstObservedAt similarly always narrows to
//     min(current, evidence.ObservedAt), so it always reflects the
//     earliest evidence ever recorded, regardless of arrival order.
//  2. Identity (Provenance, DisplayName): only replaced when
//     registry.ShouldSupersede says the new evidence outranks the current
//     source (see precedence.go). Weaker evidence arriving after stronger
//     evidence never downgrades a service's recorded identity.
//
// Both reductions are commutative and associative over the evidence
// stream, so the final state is identical regardless of the order Record
// is called in -- verified in store_test.go.
func (s *Store) Record(evidence Evidence) error {
	if evidence.ServiceName == "" {
		return fmt.Errorf("registry: service name must not be empty")
	}
	if _, ok := precedence[evidence.Source]; !ok {
		return fmt.Errorf("registry: unknown evidence source %q", evidence.Source)
	}

	existing, ok, err := s.Get(evidence.ServiceName)
	if err != nil {
		return fmt.Errorf("registry: record %s: %w", evidence.ServiceName, err)
	}

	ts := formatTime(evidence.ObservedAt)
	// LastTelemetryAt specifically means "real OTel telemetry," not "any
	// evidence" -- a hypothetical DECLARED-only service (no source
	// implemented for it yet) would have no telemetry at all. Only
	// OBSERVED_TELEMETRY evidence advances it; see EvaluateLifecycle's own
	// doc comment for the resulting, deliberate scope boundary (a
	// telemetry-less service never goes STALE/RETIRED under today's
	// lifecycle sweep).
	var telemetryTS *string
	if evidence.Source == ProvenanceObservedTelemetry {
		telemetryTS = &ts
	}

	if !ok {
		// First evidence ever for this name: it is, trivially, both the
		// most recent AND the most authoritative evidence seen so far, so
		// authority_observed_at starts equal to last_observed_at.
		_, err := s.db.Exec(`
			INSERT INTO services (name, display_name, provenance, status, first_observed_at, last_observed_at, last_telemetry_at, authority_observed_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, evidence.ServiceName, evidence.ServiceName, string(evidence.Source), string(StatusActive), ts, ts, telemetryTS, ts, ts, ts)
		if err != nil {
			return fmt.Errorf("registry: record %s: %w", evidence.ServiceName, err)
		}
		return nil
	}

	lastObserved := evidence.ObservedAt
	if existing.LastObservedAt.After(lastObserved) {
		lastObserved = existing.LastObservedAt
	}
	firstObserved := evidence.ObservedAt
	if existing.FirstObservedAt.Before(firstObserved) {
		firstObserved = existing.FirstObservedAt
	}

	// last_telemetry_at only ever moves forward from telemetry evidence,
	// and is left exactly as it was for any other source -- never cleared,
	// never advanced by a non-telemetry sighting.
	var telemetryValue *string
	if existing.LastTelemetryAt != nil {
		existingTS := formatTime(*existing.LastTelemetryAt)
		telemetryValue = &existingTS
	}
	if telemetryTS != nil && (telemetryValue == nil || evidence.ObservedAt.After(*existing.LastTelemetryAt)) {
		telemetryValue = telemetryTS
	}

	// Compared against authorityObservedAt -- the last time the CURRENTLY
	// authoritative source was itself confirmed -- never against
	// LastObservedAt, which any weaker evidence can also advance. Using
	// LastObservedAt here would let an unrelated, non-authoritative
	// sighting corrupt the tie-break for a later equal-precedence source.
	provenance := existing.Provenance
	displayName := existing.DisplayName
	authorityObservedAt := existing.authorityObservedAt
	if ShouldSupersede(existing.Provenance, existing.authorityObservedAt, evidence.Source, evidence.ObservedAt) {
		provenance = evidence.Source
		displayName = evidence.ServiceName
		authorityObservedAt = evidence.ObservedAt
	}

	_, err = s.db.Exec(`
		UPDATE services SET
			display_name = ?, provenance = ?, status = ?,
			first_observed_at = ?, last_observed_at = ?, last_telemetry_at = ?, authority_observed_at = ?, updated_at = ?
		WHERE name = ?
	`, displayName, string(provenance), string(StatusActive), formatTime(firstObserved), formatTime(lastObserved), telemetryValue, formatTime(authorityObservedAt), ts, evidence.ServiceName)
	if err != nil {
		return fmt.Errorf("registry: record %s: %w", evidence.ServiceName, err)
	}
	return nil
}

// Get returns the registry record for name, if any. ok is false when the
// service has never been observed -- callers must not treat a zero-value
// Service as meaningful.
func (s *Store) Get(name string) (Service, bool, error) {
	row := s.db.QueryRow(selectColumns+` FROM services WHERE name = ?`, name)
	svc, err := scanService(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Service{}, false, nil
	}
	if err != nil {
		return Service{}, false, fmt.Errorf("registry: get %s: %w", name, err)
	}
	return svc, true, nil
}

// ListFilter narrows List's results. A nil/empty field means "no filter on
// this dimension" -- never a filter that happens to match everything by
// coincidence. Deliberately small: status/source/name-substring only, no
// pagination or full-text search (see docs/registry.md's discussion of
// why that's out of scope for a registry at this project's real scale).
type ListFilter struct {
	Status *Status
	Source *Provenance
	// Query matches services whose name contains this substring,
	// case-insensitively. Empty string matches everything.
	Query string
}

// List returns every known service matching filter, alphabetically by
// name -- ACTIVE, STALE, and RETIRED alike unless Status narrows that.
// Retired services are never deleted, so an unfiltered List always
// includes them; the frontend/API caller decides how to present that.
// Ordering is always by name, regardless of which filters are set, so
// results are deterministic across repeated calls.
func (s *Store) List(filter ListFilter) ([]Service, error) {
	query := selectColumns + ` FROM services WHERE 1=1`
	args := make([]any, 0, 3)

	if filter.Status != nil {
		query += ` AND status = ?`
		args = append(args, string(*filter.Status))
	}
	if filter.Source != nil {
		query += ` AND provenance = ?`
		args = append(args, string(*filter.Source))
	}
	if filter.Query != "" {
		// Matched against lower(name) explicitly for case-insensitivity,
		// rather than relying on SQLite's default LIKE behavior (which is
		// only ASCII-case-insensitive and not guaranteed by this driver's
		// build configuration).
		query += ` AND lower(name) LIKE ? ESCAPE '\'`
		args = append(args, `%`+escapeLike(strings.ToLower(filter.Query))+`%`)
	}
	query += ` ORDER BY name`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("registry: list: %w", err)
	}
	defer rows.Close()

	services := make([]Service, 0)
	for rows.Next() {
		svc, err := scanService(rows)
		if err != nil {
			return nil, fmt.Errorf("registry: list: %w", err)
		}
		services = append(services, svc)
	}
	return services, rows.Err()
}

// escapeLike escapes SQLite LIKE metacharacters in user-supplied search
// text so a query string doesn't behave as a SQL wildcard.
func escapeLike(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}

// EvaluateLifecycle applies the deterministic ACTIVE -> STALE -> RETIRED
// transitions based on elapsed time since each service's LastTelemetryAt.
// It never moves a service backward (only Record does that, on real
// evidence) and never deletes a row -- a RETIRED service's identity and
// full timestamp history remain queryable indefinitely. Called
// periodically from the same background loop as the rest of Atlas's
// cleanup passes (see cmd/intelligence-engine/main.go); requires
// retireAfter > staleAfter (validated at startup) so a service cannot skip
// straight from ACTIVE to RETIRED in one evaluation.
//
// Deliberate Phase 7C scope boundary: this keys strictly on
// LastTelemetryAt, not LastObservedAt, so a hypothetical service known only
// through a future non-telemetry source (DECLARED, say) -- one that has
// never actually emitted OTel telemetry -- would never transition out of
// ACTIVE under this query (its last_telemetry_at is NULL, excluded by both
// WHERE clauses below). Designing lifecycle semantics for a source that
// doesn't exist yet would be speculative; this is documented as a known
// limitation to revisit once a real non-telemetry source is implemented,
// not silently patched over now.
func (s *Store) EvaluateLifecycle(now time.Time, staleAfter, retireAfter time.Duration) error {
	nowStr := formatTime(now)
	staleCutoff := formatTime(now.Add(-staleAfter))
	retireCutoff := formatTime(now.Add(-retireAfter))

	// Retirement is evaluated first and matches ACTIVE or STALE rows whose
	// last telemetry is already older than retireCutoff. This makes the
	// result a pure function of elapsed time since last telemetry, not of
	// how many prior EvaluateLifecycle calls happened to run in between --
	// a service quiet for far longer than retireAfter retires in a single
	// call, exactly as it would after many periodic ticks in production.
	if _, err := s.db.Exec(
		`UPDATE services SET status = ?, updated_at = ? WHERE status IN (?, ?) AND last_telemetry_at IS NOT NULL AND last_telemetry_at < ?`,
		string(StatusRetired), nowStr, string(StatusActive), string(StatusStale), retireCutoff,
	); err != nil {
		return fmt.Errorf("registry: evaluate lifecycle (retire): %w", err)
	}

	// Anything still ACTIVE past staleCutoff (and therefore NOT already
	// caught by the retire step above, since retireAfter > staleAfter is
	// enforced at startup) becomes STALE.
	if _, err := s.db.Exec(
		`UPDATE services SET status = ?, updated_at = ? WHERE status = ? AND last_telemetry_at IS NOT NULL AND last_telemetry_at < ?`,
		string(StatusStale), nowStr, string(StatusActive), staleCutoff,
	); err != nil {
		return fmt.Errorf("registry: evaluate lifecycle (stale): %w", err)
	}

	return nil
}

const selectColumns = `SELECT name, display_name, provenance, status, first_observed_at, last_observed_at, last_telemetry_at, authority_observed_at, created_at, updated_at`

type scanner interface {
	Scan(dest ...any) error
}

func scanService(row scanner) (Service, error) {
	var svc Service
	var provenance, status, firstObserved, lastObserved, created, updated, authorityObserved string
	var lastTelemetry sql.NullString

	if err := row.Scan(&svc.Name, &svc.DisplayName, &provenance, &status, &firstObserved, &lastObserved, &lastTelemetry, &authorityObserved, &created, &updated); err != nil {
		return Service{}, err
	}

	svc.Provenance = Provenance(provenance)
	svc.Status = Status(status)

	var err error
	if svc.FirstObservedAt, err = parseTime(firstObserved); err != nil {
		return Service{}, fmt.Errorf("first_observed_at: %w", err)
	}
	if svc.LastObservedAt, err = parseTime(lastObserved); err != nil {
		return Service{}, fmt.Errorf("last_observed_at: %w", err)
	}
	if svc.authorityObservedAt, err = parseTime(authorityObserved); err != nil {
		return Service{}, fmt.Errorf("authority_observed_at: %w", err)
	}
	if svc.CreatedAt, err = parseTime(created); err != nil {
		return Service{}, fmt.Errorf("created_at: %w", err)
	}
	if svc.UpdatedAt, err = parseTime(updated); err != nil {
		return Service{}, fmt.Errorf("updated_at: %w", err)
	}
	if lastTelemetry.Valid {
		t, err := parseTime(lastTelemetry.String)
		if err != nil {
			return Service{}, fmt.Errorf("last_telemetry_at: %w", err)
		}
		svc.LastTelemetryAt = &t
	}

	return svc, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}
