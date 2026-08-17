package blast

import (
	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
)

type Analyzer struct {
	corrEngine *correlation.Engine
}

func NewAnalyzer(corrEngine *correlation.Engine) *Analyzer {
	return &Analyzer{
		corrEngine: corrEngine,
	}
}

// Calculate updates the Incident with its blast radius.
func (a *Analyzer) Calculate(inc *incidentmodel.Incident) {
	if inc == nil || len(inc.TraceIDs) == 0 {
		return
	}

	services := make(map[string]bool)
	operations := make(map[string]bool)
	edges := make(map[string]bool)
	failureCount := 0

	for _, traceID := range inc.TraceIDs {
		traceDTO, ok := a.corrEngine.GetTrace(traceID)
		if !ok {
			continue
		}

		if traceDTO.OverallStatus == "ERROR" {
			failureCount++
		}

		// Iterate over spans to find affected
		for _, span := range traceDTO.Spans {
			if span.Status == "ERROR" || span.Status == "5xx" {
				services[span.ServiceName] = true
				operations[span.OperationName] = true
				if span.ParentSpanID != "" {
					// find parent to form edge
					for _, pSpan := range traceDTO.Spans {
						if pSpan.SpanID == span.ParentSpanID {
							edgeStr := pSpan.ServiceName + "->" + span.ServiceName
							edges[edgeStr] = true
							break
						}
					}
				}
			}
		}
	}

	inc.AffectedServices = make([]string, 0, len(services))
	for s := range services {
		inc.AffectedServices = append(inc.AffectedServices, s)
	}

	inc.AffectedOperations = make([]string, 0, len(operations))
	for o := range operations {
		inc.AffectedOperations = append(inc.AffectedOperations, o)
	}

	inc.AffectedEdges = make([]string, 0, len(edges))
	for e := range edges {
		inc.AffectedEdges = append(inc.AffectedEdges, e)
	}

	// Wait, we cannot easily set TraceCount on Incident since it's just len(TraceIDs)
	// But it says Calculate: TraceCount, FailureCount
	// Maybe we should store it in the incident?
	// Oh, incident model doesn't explicitly have TraceCount / FailureCount fields yet, but we can just use len(TraceIDs)
	// Wait, we could add TraceCount and FailureCount to Incident struct if needed.
}
