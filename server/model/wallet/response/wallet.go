package response

type Balance struct {
	// AccountNo和Phone仅由后台查询接口填充；客户端余额接口不暴露这些字段。
	AccountNo        string `json:"accountNo,omitempty"`
	Phone            string `json:"phone,omitempty"`
	CurrencyCode     string `json:"currencyCode"`
	AvailableBalance string `json:"availableBalance"`
	FrozenBalance    string `json:"frozenBalance"`
	Version          string `json:"version"`
	Status           string `json:"status"`
	UpdatedAt        int64  `json:"updatedAt"`
}
type Ledger struct {
	LedgerNo         string `json:"ledgerNo"`
	CurrencyCode     string `json:"currencyCode"`
	WalletVersion    string `json:"walletVersion"`
	ChangeAmount     string `json:"changeAmount"`
	BalanceBefore    string `json:"balanceBefore"`
	BalanceAfter     string `json:"balanceAfter"`
	BusinessType     string `json:"businessType"`
	BusinessNo       string `json:"businessNo"`
	OriginalLedgerNo string `json:"originalLedgerNo,omitempty"`
	CreatedAt        int64  `json:"createdAt"`
	// 内部备注、凭证和管理员身份只在管理端提供。
	OperatorID      string `json:"operatorId,omitempty"`
	OperatorDisplay string `json:"operatorDisplay,omitempty"`
	Reason          string `json:"reason,omitempty"`
}
type LedgerList struct {
	List       []Ledger `json:"list"`
	NextCursor string   `json:"nextCursor"`
	HasMore    bool     `json:"hasMore"`
}
type Order struct {
	OrderNo           string `json:"orderNo"`
	RequestID         string `json:"requestId"`
	SourceType        string `json:"sourceType"`
	ExternalOrderNo   string `json:"externalOrderNo"`
	Kind              string `json:"kind"`
	AccountNo         string `json:"accountNo"`
	CurrencyCode      string `json:"currencyCode"`
	Amount            string `json:"amount"`
	Reason            string `json:"reason"`
	EvidenceRef       string `json:"evidenceRef"`
	Status            string `json:"status"`
	OriginalOrderNo   string `json:"originalOrderNo,omitempty"`
	ReversalOrderNo   string `json:"reversalOrderNo,omitempty"`
	LedgerNo          string `json:"ledgerNo,omitempty"`
	CreatedBy         string `json:"createdBy"`
	CreatedByDisplay  string `json:"createdByDisplay,omitempty"`
	ApprovedBy        string `json:"approvedBy,omitempty"`
	ApprovedByDisplay string `json:"approvedByDisplay,omitempty"`
	RejectedBy        string `json:"rejectedBy,omitempty"`
	RejectedByDisplay string `json:"rejectedByDisplay,omitempty"`
	ReviewReason      string `json:"reviewReason"`
	CreatedIP         string `json:"createdIp"`
	ReviewedIP        string `json:"reviewedIp"`
	CreatedAt         int64  `json:"createdAt"`
	UpdatedAt         int64  `json:"updatedAt"`
	ApprovedAt        int64  `json:"approvedAt"`
	CreditedAt        int64  `json:"creditedAt"`
	RejectedAt        int64  `json:"rejectedAt"`
	ReversedAt        int64  `json:"reversedAt"`
}
type Audit struct {
	Action          string `json:"action"`
	OperatorID      string `json:"operatorId"`
	OperatorDisplay string `json:"operatorDisplay"`
	OperatorIP      string `json:"operatorIp"`
	Reason          string `json:"reason"`
	CreatedAt       int64  `json:"createdAt"`
}
type OrderDetail struct {
	Order  Order   `json:"order"`
	Ledger *Ledger `json:"ledger"`
	Audit  []Audit `json:"audit"`
}
type Check struct {
	AccountNo       string   `json:"accountNo"`
	CurrencyCode    string   `json:"currencyCode"`
	Consistent      bool     `json:"consistent"`
	Issues          []string `json:"issues"`
	Checked         int      `json:"checked"`
	NextVersion     string   `json:"nextVersion"`
	HasMore         bool     `json:"hasMore"`
	NextOrderID     string   `json:"nextOrderId"`
	CheckedOrders   int      `json:"checkedOrders"`
	SnapshotVersion string   `json:"snapshotVersion"`
}
