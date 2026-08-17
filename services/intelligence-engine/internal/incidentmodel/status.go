package incidentmodel

type Status string

const (
	StatusOpen         Status = "OPEN"
	StatusAcknowledged Status = "ACKNOWLEDGED"
	StatusResolved     Status = "RESOLVED"
)
