package config

// LivePresence bounds are applied even when a YAML file contains unsafe values.
// Credentials derive from the frozen APP key with a dedicated HMAC domain.
type LivePresence struct {
	HeartbeatSeconds  int `mapstructure:"heartbeat-seconds" json:"heartbeat-seconds" yaml:"heartbeat-seconds"`
	TTLSeconds        int `mapstructure:"ttl-seconds" json:"ttl-seconds" yaml:"ttl-seconds"`
	RecheckSeconds    int `mapstructure:"recheck-seconds" json:"recheck-seconds" yaml:"recheck-seconds"`
	RoomConnections   int `mapstructure:"room-connections" json:"room-connections" yaml:"room-connections"`
	GlobalConnections int `mapstructure:"global-connections" json:"global-connections" yaml:"global-connections"`
	IPConnections     int `mapstructure:"ip-connections" json:"ip-connections" yaml:"ip-connections"`
	LikeSecond        int `mapstructure:"like-second" json:"like-second" yaml:"like-second"`
	LikeMinute        int `mapstructure:"like-minute" json:"like-minute" yaml:"like-minute"`
}

func (c LivePresence) Bounds() LivePresence {
	bound := func(n, d, lo, hi int) int {
		if n == 0 {
			return d
		}
		return max(lo, min(n, hi))
	}
	c.HeartbeatSeconds = bound(c.HeartbeatSeconds, 15, 10, 20)
	c.TTLSeconds = bound(c.TTLSeconds, 45, c.HeartbeatSeconds*3, 90)
	c.RecheckSeconds = bound(c.RecheckSeconds, 30, 10, 60)
	c.RoomConnections = bound(c.RoomConnections, 10000, 4, 50000)
	c.GlobalConnections = bound(c.GlobalConnections, 100000, 100, 500000)
	c.IPConnections = bound(c.IPConnections, 300, 4, 2000)
	c.LikeSecond = bound(c.LikeSecond, 10, 1, 20)
	c.LikeMinute = bound(c.LikeMinute, 120, c.LikeSecond, 300)
	return c
}
