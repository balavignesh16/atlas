// Package replay implements Module 5: a read-only "replay/simulation"
// capability that re-runs ATLAS's advisory analysis (AI reasoning,
// remediation plan generation) against an existing incident's currently-
// still-available persisted context, without ever approving or executing
// anything.
//
// This package imports two of the six frozen packages, both read-only and
// unmodified: internal/rca, solely for its plain RCACandidate struct (the
// exact same type internal/aireasoning.IncidentAnalysisContext already
// depends on -- reusing an existing exported type is not a package
// modification); and internal/remediation, for its already-exported
// Planner/Config/RemediationPlannerProvider/RemediationPlan -- this package
// never calls Planner.ApprovePlan. internal/execution is never imported at
// all, anywhere in this package or its callers, which is what makes it
// structurally impossible for replay to execute or approve anything: there
// is no code path here that could even attempt it.
//
// Why rca.Engine is never re-invoked: rca.Engine.Analyze (frozen) reads
// LIVE internal/evidence and internal/graph state at call time, not a
// frozen historical snapshot -- re-running it "at replay time" would
// silently substitute whatever evidence/graph state exists NOW for the
// state that actually existed when the incident was first detected, which
// would be dishonest to present as "the historical RCA." Instead, replay
// reads the incident's own already-computed RCA fields (RCA, Confidence,
// Score, DetectionReason) as historical fact -- exactly how
// internal/httpapi's existing handleGetRCA already treats them.
//
// Why AI reasoning and remediation planning CAN be safely re-invoked:
// aireasoning.Engine.Analyze and remediation.Planner.GeneratePlan are both
// pure functions of their arguments (plus their own private, unshared
// internal cache) -- they never reach into a live global store on their
// own. Replay constructs a FRESH, throwaway Engine/Planner instance per
// request (see simulator.go), wrapping the SAME underlying provider objects
// production uses, so its own analysis/plan can never be written into
// production's real, shared analysis cache or plan store.
package replay

import (
	"time"

	"github.com/atlas/intelligence-engine/internal/aireasoning"
	"github.com/atlas/intelligence-engine/internal/correlationmodel"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/remediation"
)

// HistoricalRCA is the incident's own already-computed RCA output, read
// verbatim -- never re-computed. Available is false only when the incident
// has genuinely never had RCA run against it yet (RCA == nil on the source
// incident), matching the same honest UNKNOWN/NONE convention
// internal/httpapi's handleGetRCA already uses.
type HistoricalRCA struct {
	Available       bool
	Service         string
	Confidence      string
	Score           int
	DetectionReason string
}

// EvidenceContext is a best-effort snapshot of whatever evidence the
// incident's own EvidenceIDs still resolve to in the live evidence store.
// Found can be less than Requested if some evidence has already expired
// under internal/evidence's own retention -- that gap is reported
// honestly, never backfilled with a fabricated value.
type EvidenceContext struct {
	Requested int
	Found     int
	Evidence  []*evidence.Evidence
}

// DependencyContext is the live dependency graph's current edge snapshot,
// passed to AI reasoning exactly the same way internal/httpapi's real
// POST /analyze already does (the whole live graph, not a subset scoped to
// this incident's services specifically) -- replay does not introduce a new
// filtering behavior AI reasoning doesn't already have in production.
type DependencyContext struct {
	EdgeCount int
	Edges     []*correlationmodel.DependencyEdge
}

// AIReplayOutcome distinguishes "never attempted," "attempted but honestly
// could not complete" (disabled, or the real evidence-grounding validator
// rejected an ungrounded result), and "succeeded."
type AIReplayOutcome struct {
	Attempted bool
	Succeeded bool
	Reason    string                      // populated only when Succeeded is false
	Result    *aireasoning.AnalysisResult // nil unless Succeeded
}

// PlanReplayOutcome mirrors AIReplayOutcome for remediation plan
// generation. A generated plan here is for inspection only: replay never
// calls Planner.ApprovePlan, and no code in this package or its callers
// references internal/execution at all.
type PlanReplayOutcome struct {
	Attempted bool
	Succeeded bool
	Reason    string
	Plan      *remediation.RemediationPlan
}

// Result is Module 5's full simulation output. Simulation, ExecutionPerformed,
// and ApprovalPerformed are always true/false/false respectively -- fixed,
// non-configurable fields whose only purpose is to make it structurally
// impossible for a caller to mistake this response for a real execution
// result, even by careless field-name confusion.
//
// HistoricalAnalysis/HistoricalPlan are read-only lookups against
// production's own real, shared stores (aireasoning.Engine.GetAnalysis,
// remediation.Planner.GetPlanByIncident) -- nil when nothing was ever
// really triggered for this incident, never fabricated. Where both a
// historical and a replay value exist, the caller can compare them
// directly; this package does not compute or assert a "changed" verdict,
// since that would mean picking which fields matter to compare, and no
// single choice is justified by the current architecture.
type Result struct {
	ReplayID           string
	SourceIncidentID   string
	ReplayTimestamp    time.Time
	Simulation         bool
	ExecutionPerformed bool
	ApprovalPerformed  bool

	HistoricalRCA HistoricalRCA
	Evidence      EvidenceContext
	Dependencies  DependencyContext

	ReplayAnalysis     AIReplayOutcome
	HistoricalAnalysis *aireasoning.AnalysisResult

	ReplayPlan     PlanReplayOutcome
	HistoricalPlan *remediation.RemediationPlan
}
