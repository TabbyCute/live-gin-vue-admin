package eventdelivery

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	model "tb_live_module/model/eventdelivery"
)

// Validate again immediately before transport: DB corruption cannot turn this
// service into a raw payload/secret exfiltration channel. Unknown fields fail closed.
func validEnvelope(e model.Event) bool {
	if e.SchemaVersion != 1 || e.EventID != stableID(e.Source, e.SourceID) || e.PayloadHash != hashPayload(e.Payload) || len(e.Payload) > 2048 || e.OccurredAt <= 0 {
		return false
	}
	var fields map[string]string
	dec := json.NewDecoder(bytes.NewBufferString(e.Payload))
	if dec.Decode(&fields) != nil {
		return false
	}
	if dec.Decode(new(any)) != io.EOF {
		return false
	}
	if e.Source == "payout" {
		id, err := strconv.ParseUint(e.Destination, 10, 64)
		return err == nil && id > 0 && strconv.FormatUint(id, 10) == e.Destination && e.Audience == "user" && len(fields) == 2 && validNo(fields["payoutNo"], 40) && fields["status"] == e.EventType && (e.EventType == "payout.succeeded" || e.EventType == "payout.cancelled")
	}
	if e.Audience != "room" || !validNo(fields["roomNo"], 64) || !validNo(fields["sessionNo"], 64) || e.Destination != fields["sessionNo"] {
		return false
	}
	if e.Source == "gift" {
		return e.EventType == "gift.sent" && len(fields) == 3 && validNo(fields["orderNo"], 40)
	}
	return e.Source == "presence" && roomTypes[e.EventType] && len(fields) == 2
}
