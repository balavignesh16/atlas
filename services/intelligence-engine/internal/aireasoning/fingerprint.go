package aireasoning

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/atlas/intelligence-engine/internal/incidentmodel"
)

// GenerateFingerprint generates a unique hash representing the current deterministic state
// of an incident, including its evidence count and last update time.
func GenerateFingerprint(incident *incidentmodel.Incident) string {
	if incident == nil {
		return ""
	}

	// We consider the incident fundamentally changed if its evidence count or last updated time changes.
	raw := fmt.Sprintf("%s|%d|%d", incident.IncidentID, len(incident.EvidenceIDs), incident.LastUpdatedAt.UnixNano())
	
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}
