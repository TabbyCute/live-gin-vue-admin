package request

import common "tb_live_module/model/common/request"

type Import struct {
	RequestID        string `json:"requestId"`
	ChannelAccountNo string `json:"channelAccountNo"`
	PeriodFrom       int64  `json:"periodFrom"`
	PeriodTo         int64  `json:"periodTo"`
	DeclaredComplete bool   `json:"declaredComplete"`
	CSV              string `json:"csv"`
}
type Run struct {
	RequestID string `json:"requestId"`
	BatchNo   string `json:"batchNo"`
}
type Review struct {
	RequestID string `json:"requestId"`
	CheckNo   string `json:"checkNo"`
	Decision  string `json:"decision"`
	Reason    string `json:"reason"`
}
type Search struct {
	common.PageInfo
	ChannelAccountNo string `form:"channelAccountNo" json:"channelAccountNo"`
}
