package remediation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/atlas/intelligence-engine/internal/aireasoning"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
)

// GenerateFingerprint generates a unique hash representing the deterministic state
// of an incident + M2.4 RCA + M2.5 AI Analysis.
func GenerateFingerprint(incident *incidentmodel.Incident, analysis *aireasoning.AnalysisResult) string {
	if incident == nil {
		return ""
	}

	analysisHash := ""
	if analysis != nil {
		analysisHash = analysis.Fingerprint
	}

	rcaHash := "none"
	if incident.RCA != nil {
		rcaHash = fmt.Sprintf("%s|%s|%d", incident.RCA.Service, incident.RCA.Confidence, incident.RCA.Score)
	}

	raw := fmt.Sprintf("%s|%d|%d|%s|%s", 
		incident.IncidentID, 
		len(incident.EvidenceIDs), 
		incident.LastUpdatedAt.UnixNano(),
		rcaHash,
		analysisHash,
	)
	
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}
