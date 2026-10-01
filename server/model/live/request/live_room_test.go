package request

import (
	"encoding/json"
	"testing"
)

func TestSRSHookReqUnmarshalSRS6FieldsAndKeepUnknownFields(t *testing.T) {
	const body = `{
		"server_id":"vid-o154223",
		"service_id":"vid-n9a8b7c6",
		"action":"on_publish",
		"client_id":"0t95647z",
		"ip":"192.0.2.10",
		"vhost":"push.example.com",
		"app":"tb_live",
		"tcUrl":"rtmp://push.example.com/tb_live",
		"stream":"A_1000009",
		"param":"?pt=v1_example",
		"stream_url":"/tb_live/A_1000009",
		"stream_id":"vid-21w91p4",
		"protocol":"rtmp"
	}`

	var req SRSHookReq
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal SRS hook request: %v", err)
	}

	if req.ServerID != "vid-o154223" || req.ServiceID != "vid-n9a8b7c6" {
		t.Fatalf("unexpected server identity: server_id=%q service_id=%q", req.ServerID, req.ServiceID)
	}
	if req.Action != "on_publish" || req.ClientID != "0t95647z" {
		t.Fatalf("unexpected callback identity: action=%q client_id=%q", req.Action, req.ClientID)
	}
	if req.TCURL != "rtmp://push.example.com/tb_live" || req.StreamURL != "/tb_live/A_1000009" {
		t.Fatalf("unexpected stream URL fields: tcUrl=%q stream_url=%q", req.TCURL, req.StreamURL)
	}
	if req.Stream != "A_1000009" || req.StreamID != "vid-21w91p4" || req.Param != "?pt=v1_example" {
		t.Fatalf("unexpected stream fields: stream=%q stream_id=%q param=%q", req.Stream, req.StreamID, req.Param)
	}
	if got := string(req.Extra["protocol"]); got != `"rtmp"` {
		t.Fatalf("unknown field was not preserved, got %q", got)
	}
}
