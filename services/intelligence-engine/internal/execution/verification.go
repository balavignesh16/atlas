package execution

import (
	"context"
	"log"
	"time"

	"github.com/atlas/intelligence-engine/internal/incidentmanager"
)

type Verifier struct {
	manager *incidentmanager.Manager
}

func NewVerifier(manager *incidentmanager.Manager) *Verifier {
	return &Verifier{manager: manager}
}

// Verify implements post-execution verification using the M2.4 source of truth.
func (v *Verifier) Verify(ctx context.Context, incidentID string, serviceName string) VerificationStatus {
	log.Printf("[INFO] Verifying execution outcome for incident %s, service %s", incidentID, serviceName)

	// Since M2.7 is not a decision engine and M2.4 remains the source of truth,
	// we query the Incident Manager to see if the incident has naturally resolved
	// or if the error rate remains elevated.

	// In a real system, we'd poll or wait for the M2.4 evaluation cycle to finish.
	// We do a simple loop here:
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[WARN] Verification timed out for incident %s", incidentID)
			return VerificationFailed
		case <-ticker.C:
			inc := v.manager.GetIncident(incidentID)
			if inc == nil {
				// Incident resolved or disappeared -> verified!
				return VerificationVerified
			}

			if inc.Status == "RESOLVED" {
				return VerificationVerified
			}

			// If it's still open, verification hasn't succeeded yet. We keep waiting.
			// The timeout will eventually fail it.
		}
	}
}
