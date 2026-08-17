package action

const (
	TypeRestartService        ActionType = "RESTART_SERVICE"
	TypeReduceTraffic         ActionType = "REDUCE_TRAFFIC"
	TypeRestoreTraffic        ActionType = "RESTORE_TRAFFIC"
	TypeRollbackDeployment    ActionType = "ROLLBACK_DEPLOYMENT"
	TypeScaleService          ActionType = "SCALE_SERVICE"
	TypeClearConnectionPool   ActionType = "CLEAR_CONNECTION_POOL"
	TypeObserve               ActionType = "OBSERVE"
	TypeInvestigate           ActionType = "INVESTIGATE"
)

var Catalog = map[ActionType]bool{
	TypeRestartService:        true,
	TypeReduceTraffic:         true,
	TypeRestoreTraffic:        true,
	TypeRollbackDeployment:    true,
	TypeScaleService:          true,
	TypeClearConnectionPool:   true,
	TypeObserve:               true,
	TypeInvestigate:           true,
}

// IsValid checks if the provided ActionType exists in the allowed catalog.
func IsValid(actionType ActionType) bool {
	return Catalog[actionType]
}
