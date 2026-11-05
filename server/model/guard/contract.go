package guard

// These commands cross the gift/guard boundary. Guard never reads gift tables.
type GrantInput struct {
	GrantNo         string `json:"grantNo"`
	GiftOrderNo     string `json:"giftOrderNo"`
	UserID          uint64 `json:"userId"`
	AnchorID        uint64 `json:"anchorId"`
	PlanNo          string `json:"planNo"`
	DurationDays    int64  `json:"durationDays"`
	BenefitSnapshot string `json:"benefitSnapshot"`
}
type CompensationInput struct {
	Grant          GrantInput `json:"grant"`
	CompensationNo string     `json:"compensationNo"`
	RefundNo       string     `json:"refundNo"`
	DurationDays   int64      `json:"durationDays"`
}

// PlanSnapshot is immutable and includes only entitlement configuration.
type PlanSnapshot struct {
	PlanNo        string `json:"planNo"`
	Name          string `json:"name"`
	Level         int    `json:"level"`
	Priority      int    `json:"priority"`
	BenefitConfig string `json:"benefitConfig"`
	Version       int64  `json:"version,string"`
}
type BenefitSnapshot struct {
	BenefitType       string       `json:"benefitType"`
	GuardPlanNo       string       `json:"guardPlanNo"`
	GuardDurationDays string       `json:"guardDurationDays"`
	TotalDurationDays string       `json:"totalDurationDays"`
	Status            string       `json:"status"`
	Plan              PlanSnapshot `json:"plan"`
}
