package payout

var orderTransitions = map[string]map[string]bool{
	"ready":                {"processing": true, "cancelled": true},
	"processing":           {"ready": true, "pending_confirmation": true, "failed": true},
	"pending_confirmation": {"succeeded": true, "failed": true, "unknown": true},
	"unknown":              {"succeeded": true, "failed": true},
	"failed":               {"processing": true, "cancelled": true},
}

func ValidOrderStatus(status string) bool {
	return status == "succeeded" || status == "cancelled" || orderTransitions[status] != nil
}
