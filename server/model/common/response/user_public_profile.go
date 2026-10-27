package response

// UserPublicProfile 是 APP 公开用户资料的唯一契约。禁止嵌入账号数据库模型。
// IsLive 仅表示当前访问者可进入的有效直播；离线或无权访问时不返回直播入口。
type UserPublicProfile struct {
	AccountNo   string `json:"accountNo"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	Signature   string `json:"signature"`
	Gender      uint8  `json:"gender"`
	CountryCode string `json:"countryCode"`
	AnchorNo    string `json:"anchorNo,omitempty"`
	IsLive      bool   `json:"isLive"`
	RoomNo      string `json:"roomNo,omitempty"`
	SessionNo   string `json:"sessionNo,omitempty"`
}
