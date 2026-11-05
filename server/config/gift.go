package config

import "fmt"

// Gift controls new guard-bearing sales. Existing grants/refunds are processed
// even with sales disabled; enabling requires runtime readiness verification.
type Gift struct {
	GuardEnabled bool `mapstructure:"guard-enabled" json:"guardEnabled" yaml:"guard-enabled"`
}

func (c Gift) Validate(ready ...bool) error {
	if c.GuardEnabled && (len(ready) != 1 || !ready[0]) {
		return fmt.Errorf("gift guard-enabled requires verified guard schema, plans, provider and workers")
	}
	return nil
}
