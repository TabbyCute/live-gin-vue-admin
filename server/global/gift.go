package global

import "sync/atomic"

// Set only after gift schema verification at startup. Never substitute legacy SRS amounts.
var GiftFactsReady atomic.Bool
var GuardReady atomic.Bool
var RankingReady atomic.Bool
