package config

// AnchorHome is local private media storage; never expose this directory via a static web route.
type AnchorHome struct {
	PublicReadPerMinute int    `mapstructure:"public-read-per-minute" json:"publicReadPerMinute" yaml:"public-read-per-minute"`
	MediaReadPerMinute  int    `mapstructure:"media-read-per-minute" json:"mediaReadPerMinute" yaml:"media-read-per-minute"`
	Disabled            bool   `mapstructure:"disabled" json:"disabled" yaml:"disabled"`
	StorageDir          string `mapstructure:"storage-dir" json:"-" yaml:"storage-dir"`
	MaxPhotos           int    `mapstructure:"max-photos" json:"maxPhotos" yaml:"max-photos"`
	MaxUploadsPerDay    int    `mapstructure:"max-uploads-per-day" json:"maxUploadsPerDay" yaml:"max-uploads-per-day"`
	ImageConcurrency    int    `mapstructure:"image-concurrency" json:"imageConcurrency" yaml:"image-concurrency"`
}
