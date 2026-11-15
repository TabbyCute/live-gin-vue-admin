// Package eventcapture is the narrow optional transactional notification boundary.
// Startup installs a recorder only after schema verification. Old applications and
// deployments with delivery disabled continue to write their existing durable facts.
package eventcapture

import (
	"gorm.io/gorm"
	"sync/atomic"
	gift "tb_live_module/model/gift"
	live "tb_live_module/model/live"
	payout "tb_live_module/model/payout"
)

type Recorder interface {
	Gift(*gorm.DB, gift.Event) error
	Payout(*gorm.DB, payout.TerminalEvent, uint64) error
	Room(*gorm.DB, []live.LiveRoomEvent) error
}
type holder struct{ recorder Recorder }

var current atomic.Pointer[holder]

// Install is a startup/shutdown hook. Call before accepting writes or after draining requests.
func Install(r Recorder) {
	if r == nil {
		current.Store(nil)
	} else {
		current.Store(&holder{r})
	}
}
func Gift(tx *gorm.DB, event gift.Event) error {
	if r := current.Load(); r != nil {
		return r.recorder.Gift(tx, event)
	}
	return nil
}
func Payout(tx *gorm.DB, event payout.TerminalEvent, userID uint64) error {
	if r := current.Load(); r != nil {
		return r.recorder.Payout(tx, event, userID)
	}
	return nil
}
func Room(tx *gorm.DB, events []live.LiveRoomEvent) error {
	if r := current.Load(); r != nil {
		return r.recorder.Room(tx, events)
	}
	return nil
}
