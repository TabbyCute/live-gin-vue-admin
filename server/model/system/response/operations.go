package response

import "tb_live_module/internal/telemetry"

type OperationBacklog struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	Count     int    `json:"count"`
	Truncated bool   `json:"truncated"`
	OldestAt  int64  `json:"oldestAt"`
}
type OperationAlert struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}
type Operations struct {
	At           int64              `json:"at"`
	Database     string             `json:"database"`
	Redis        string             `json:"redis"`
	Goroutines   int                `json:"goroutines"`
	HeapBytes    uint64             `json:"heapBytes,string"`
	DatabasePool OperationPool      `json:"databasePool"`
	Metrics      telemetry.Snapshot `json:"metrics"`
	Backlogs     []OperationBacklog `json:"backlogs"`
	Alerts       []OperationAlert   `json:"alerts"`
}
type OperationPool struct {
	MaxOpen    int   `json:"maxOpen"`
	Open       int   `json:"open"`
	InUse      int   `json:"inUse"`
	Idle       int   `json:"idle"`
	WaitCount  int64 `json:"waitCount,string"`
	WaitMillis int64 `json:"waitMillis,string"`
}
