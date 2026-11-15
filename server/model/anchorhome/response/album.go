package response

type PublicPhoto struct {
	PhotoNo      string `json:"photoNo"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnailUrl"`
}
type Photo struct {
	PublicPhoto
	AnchorNo  string `json:"anchorNo"`
	Status    string `json:"status"`
	Version   uint64 `json:"version"`
	Sort      int    `json:"sort"`
	CreatedAt int64  `json:"createdAt"`
	Reason    string `json:"reason"`
}
type Album struct {
	Version   uint64  `json:"version"`
	MaxPhotos int     `json:"maxPhotos"`
	List      []Photo `json:"list"`
}
type AlbumResult struct {
	Version      uint64 `json:"version"`
	PhotoNo      string `json:"photoNo,omitempty"`
	PhotoVersion uint64 `json:"photoVersion,omitempty"`
}
type AlbumPage struct {
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
	Total    int64   `json:"total"`
	List     []Photo `json:"list"`
}
