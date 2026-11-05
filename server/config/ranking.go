package config

import "time"

type Ranking struct {
	Timezone string `mapstructure:"timezone" json:"timezone" yaml:"timezone"`
}

func (c Ranking) Zone() string {
	if c.Timezone == "" {
		return "Asia/Kuala_Lumpur"
	}
	return c.Timezone
}
func (c Ranking) Validate() error { _, e := time.LoadLocation(c.Zone()); return e }
