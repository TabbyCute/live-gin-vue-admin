package response

type Relationship struct {
	Following   bool `json:"following"`
	FollowsYou  bool `json:"followsYou"`
	Mutual      bool `json:"mutual"`
	Blocked     bool `json:"blocked"`
	BlockedByMe bool `json:"blockedByMe"`
}

type LiveCard struct {
	RoomNo    string `json:"roomNo"`
	SessionNo string `json:"sessionNo"`
	Title     string `json:"title"`
	CoverURL  string `json:"coverUrl"`
}

// Profile is an explicit public allowlist. Financial, contact, identity and
// moderation fields must never be embedded from the anchor/account models.
type Profile struct {
	AnchorNo       string        `json:"anchorNo"`
	AccountNo      string        `json:"accountNo"`
	Nickname       string        `json:"nickname"`
	Avatar         string        `json:"avatar"`
	Signature      string        `json:"signature"`
	Gender         uint8         `json:"gender"`
	CountryCode    string        `json:"countryCode"`
	Language       string        `json:"language"`
	Level          uint32        `json:"level"`
	Certified      bool          `json:"certified"`
	Tags           []string      `json:"tags"`
	Photos         []PublicPhoto `json:"photos"`
	FollowingCount string        `json:"followingCount"`
	FansCount      string        `json:"fansCount"`
	IsSelf         bool          `json:"isSelf"`
	Relationship   Relationship  `json:"relationship"`
	Live           *LiveCard     `json:"live"`
	GeneratedAt    int64         `json:"generatedAt"`
}

type Category struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

type Settings struct {
	Signature  string     `json:"signature"`
	CategoryID uint64     `json:"categoryId"`
	Version    string     `json:"version"`
	Categories []Category `json:"categories"`
}
