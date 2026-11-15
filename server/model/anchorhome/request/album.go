package request

type AlbumMutation struct {
	RequestID string `json:"requestId"`
	Version   uint64 `json:"version"`
}
type ReorderPhotos struct {
	AlbumMutation
	PhotoNos []string `json:"photoNos"`
}
type DeletePhoto struct {
	AlbumMutation
	PhotoNo string `json:"photoNo"`
}
type ReviewPhoto struct {
	AlbumMutation
	PhotoNo  string `json:"photoNo"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}
type AlbumSearch struct {
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Status   string `json:"status"`
	AnchorNo string `json:"anchorNo"`
}
