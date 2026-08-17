package rca

type RCACandidate struct {
	Service     string
	Operation   string
	Score       int
	Confidence  string
	EvidenceIDs []string
	Reasoning   []string
}

// Sort helpers
type ByScore []*RCACandidate

func (a ByScore) Len() int           { return len(a) }
func (a ByScore) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByScore) Less(i, j int) bool { return a[i].Score > a[j].Score } // descending
