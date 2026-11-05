package request

type Rule struct {
	Name           string `json:"name"`
	ScopeType      string `json:"scopeType"`
	ScopeNo        string `json:"scopeNo"`
	Priority       int32  `json:"priority"`
	AnchorShareBPS string `json:"anchorShareBps"`
	EffectiveFrom  int64  `json:"effectiveFrom"`
	EffectiveTo    int64  `json:"effectiveTo"`
}
type Review struct {
	RuleNo  string `json:"ruleNo"`
	Approve bool   `json:"approve"`
}

type TerminateRule struct {
	RuleNo    string `json:"ruleNo"`
	RequestID string `json:"requestId"`
	Reason    string `json:"reason"`
}

type ReviewTermination struct {
	TerminationNo string `json:"terminationNo"`
	Approve       bool   `json:"approve"`
}
