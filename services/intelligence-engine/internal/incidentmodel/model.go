package incidentmodel

import "time"

type RootCause struct {
	Service    string   `json:"service"`
	Operation  string   `json:"operation"`
	Confidence string   `json:"confidence"`
	Score      int      `json:"score"`
}

type Incident struct {
	IncidentID  string   `json:"incidentId"`
	Status      Status   `json:"status"`
	Severity    Severity `json:"severity"`
	Title       string   `json:"title"`
	Description string   `json:"description"`

	StartedAt     time.Time `json:"startedAt"`
	LastUpdatedAt time.Time `json:"lastUpdatedAt"`
	ResolvedAt    *time.Time `json:"resolvedAt,omitempty"`

	Fingerprint string `json:"fingerprint"`

	RootService   string `json:"rootService"`
	RootOperation string `json:"rootOperation"`

	AffectedServices   []string `json:"affectedServices"`
	AffectedOperations []string `json:"affectedOperations"`
	AffectedEdges      []string `json:"affectedEdges"`

	TraceCount   int `json:"traceCount"`
	FailureCount int `json:"failureCount"`

	TraceIDs    []string `json:"traceIds"`
	EvidenceIDs []string `json:"evidenceIds"`

	RCA        *RootCause `json:"rootCause,omitempty"`
	Confidence string     `json:"confidence,omitempty"`
	Score      int        `json:"score,omitempty"`

	DetectionReason string `json:"detectionReason"`
}
