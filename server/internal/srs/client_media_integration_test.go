package srs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"tb_live_module/config"
	"testing"
	"time"
)

// This opt-in test uses only the disposable no-hook SRS started by tests/srs/run-local.sh.
// Every URL is checked before contacting SRS or starting ffmpeg/ffprobe.
func localMediaURL(t *testing.T, key, scheme string) string {
	t.Helper()
	raw := os.Getenv(key)
	u, err := url.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, scheme, u.Scheme)
	require.Nil(t, u.User)
	require.Empty(t, u.RawQuery)
	require.Empty(t, u.Fragment)
	require.Empty(t, u.Path)
	host, port, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", host)
	number, err := strconv.Atoi(port)
	require.NoError(t, err)
	require.Greater(t, number, 0)
	require.Less(t, number, 65536)
	return raw
}
func TestLocalMediaSmoke(t *testing.T) {
	if os.Getenv("SRS_LOCAL_SMOKE") != "1" {
		t.Skip("run tests/srs/run-local.sh for isolated real-media test")
	}
	api := localMediaURL(t, "SRS_LOCAL_API", "http")
	rtmp := localMediaURL(t, "SRS_LOCAL_RTMP", "rtmp")
	flv := localMediaURL(t, "SRS_LOCAL_FLV", "http")
	ffmpeg, err := exec.LookPath("ffmpeg")
	require.NoError(t, err)
	ffprobe, err := exec.LookPath("ffprobe")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	client, err := NewClient(config.SRS{HTTPAPIs: api, APIRequestTimeoutSeconds: 1, SnapshotPageSize: 10, SnapshotMaxStreams: 20})
	require.NoError(t, err)
	var runtime RuntimeIdentity
	require.Eventually(t, func() bool { var err error; runtime, err = client.CurrentRuntime(ctx); return err == nil }, 8*time.Second, 100*time.Millisecond)
	initial, err := client.ListPublishers(ctx)
	require.NoError(t, err)
	require.Empty(t, initial, "never point this smoke test at an existing SRS instance")
	name := fmt.Sprintf("local_smoke_%d", time.Now().UnixNano())
	publisherCtx, stopPublisher := context.WithCancel(ctx)
	defer stopPublisher()
	var publisherLog bytes.Buffer
	publish := exec.CommandContext(publisherCtx, ffmpeg, "-hide_banner", "-loglevel", "error", "-re", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=15", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100", "-t", "30", "-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-pix_fmt", "yuv420p", "-g", "15", "-c:a", "aac", "-b:a", "64k", "-ac", "1", "-f", "flv", rtmp+"/live/"+name)
	publish.Stderr = &publisherLog
	require.NoError(t, publish.Start())
	publisherDone := make(chan error, 1)
	go func() { publisherDone <- publish.Wait() }()
	defer func() {
		stopPublisher()
		select {
		case <-publisherDone:
		case <-time.After(3 * time.Second):
			t.Error("ffmpeg process did not stop")
		}
	}()
	var observed PublisherSnapshot
	require.Eventually(t, func() bool {
		batch, e := client.ListPublishersWithRuntime(ctx)
		if e != nil {
			return false
		}
		for _, stream := range batch.Publishers {
			if stream.App == "live" && stream.Stream == name && stream.Active && stream.Width == 320 && stream.Height == 180 {
				observed = stream
				return true
			}
		}
		return false
	}, 10*time.Second, 100*time.Millisecond, "publisher did not become live")
	probeCtx, probeCancel := context.WithTimeout(ctx, 10*time.Second)
	defer probeCancel()
	probe := exec.CommandContext(probeCtx, ffprobe, "-v", "error", "-analyzeduration", "1000000", "-probesize", "1000000", "-show_entries", "stream=codec_type,codec_name,width,height,sample_rate", "-of", "json", flv+"/live/"+name+".flv")
	output, err := probe.CombinedOutput()
	require.NoError(t, err, string(output))
	var media struct {
		Streams []struct {
			CodecType     string `json:"codec_type"`
			CodecName     string `json:"codec_name"`
			Width, Height int
		} `json:"streams"`
	}
	require.NoError(t, json.Unmarshal(output, &media))
	video, audio := false, false
	for _, stream := range media.Streams {
		if stream.CodecType == "video" && stream.CodecName == "h264" && stream.Width == 320 && stream.Height == 180 {
			video = true
		}
		if stream.CodecType == "audio" && stream.CodecName == "aac" {
			audio = true
		}
	}
	require.True(t, video, "HTTP-FLV pull must expose actual H264 test video")
	require.True(t, audio, "HTTP-FLV pull must expose actual AAC tone")
	ref := StreamRef{ServerID: runtime.ServerID, StreamID: observed.StreamID, Vhost: observed.Vhost, App: observed.App, Stream: observed.Stream}
	require.NoError(t, client.StopPublisher(ctx, ref))
	require.Eventually(t, func() bool {
		streams, e := client.ListPublishers(ctx)
		if e != nil {
			return false
		}
		for _, stream := range streams {
			if stream.Stream == name && stream.Active {
				return false
			}
		}
		return true
	}, 5*time.Second, 100*time.Millisecond)
	require.NoError(t, client.StopPublisher(ctx, ref), "repeat stop must be idempotent")
	select {
	case err := <-publisherDone:
		require.Error(t, err, "kicked publisher should lose its RTMP transport")
		publisherDone <- err
	case <-time.After(5 * time.Second):
		t.Fatal("SRS kick did not disconnect ffmpeg")
	}
	t.Log("actual RTMP publish -> SRS 320x180 H264/AAC snapshot -> HTTP-FLV probe -> client kick -> absent publisher -> idempotent repeat passed")
}
