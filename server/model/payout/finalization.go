package payout

// FinalizeInput is an internal, trusted same-transaction command. Never bind it
// to HTTP: amounts and destinations are taken from the persisted Order.
type FinalizeInput struct {
	Order        Order
	AttemptNo    string
	TransitionNo string
	ActorID      uint64
	Paid         bool
	Now          int64
}
