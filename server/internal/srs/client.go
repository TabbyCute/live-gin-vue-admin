package srs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"tb_live_module/config"
)

const (
	defaultRequestTimeout = 3 * time.Second
	defaultStreamPageSize = 100
	defaultMaxStreams     = 20000
	hardMaxStreamPageSize = 5000
	hardMaxStreams        = 100000
	vhostPageSize         = 100
	maxVhostPages         = 100
	maxResponseBytes      = 16 << 20
	// SRS 7查询不存在的流或客户端时仍返回HTTP 200，并把不存在状态放在JSON code中。
	// 旧版本或反向代理也可能直接返回HTTP 404，两种形式都应按幂等的资源不存在处理。
	srsCodeStreamNotFound = 2048
	srsCodeClientNotFound = 2049
)

var ErrNotFound = errors.New("SRS资源不存在")

// Controller 只暴露直播业务真正需要的媒体查询和控制能力，避免业务层依赖SRS原始响应结构。
type Controller interface {
	ListPublishers(ctx context.Context) ([]PublisherSnapshot, error)
	StopPublisher(ctx context.Context, ref StreamRef) error
}

// RuntimeController 暴露单SRS实例身份。SRS官方保证HTTP API响应中的server在进程重启后变化；
// 业务层用它区分同一实例上的冲突连接与重启后的合法新连接。
type RuntimeController interface {
	CurrentRuntime(ctx context.Context) (RuntimeIdentity, error)
	ListPublishersWithRuntime(ctx context.Context) (PublisherBatch, error)
}

type RuntimeIdentity struct {
	ServerID  string
	ServiceID string
}

type PublisherBatch struct {
	CapturedAt int64
	FromCache  bool
	Runtime    RuntimeIdentity
	Publishers []PublisherSnapshot
}

// BatchStopController 是Controller的可选高容量扩展。调用方按返回切片下标对应refs；
// 实现必须保持单项幂等，并用固定worker限制HTTP并发，不能按流创建无界goroutine。
type BatchStopController interface {
	StopPublishers(ctx context.Context, refs []StreamRef, concurrency int) []error
}

// StreamRef 唯一描述一次需要停止的发布流。StreamID可用时优先按ID查询，
// 不可用或已经失效时退回到vhost/app/stream匹配。
type StreamRef struct {
	StreamID string
	Vhost    string
	App      string
	Stream   string
}

// PublisherSnapshot 是SRS当前发布流的安全快照，不包含推流地址、鉴权参数、IP或客户端ID。
// Width和Height都大于0时，业务层才可以把场次从准备中切换为直播中。
type PublisherSnapshot struct {
	StreamID        string
	Vhost           string
	App             string
	Stream          string
	Active          bool
	VideoCodec      string
	VideoProfile    string
	VideoLevel      string
	Width           int
	Height          int
	AudioCodec      string
	AudioProfile    string
	AudioSampleRate int
	AudioChannels   int
}

type Client struct {
	baseURL        string
	username       string
	password       string
	http           *http.Client
	streamPageSize int
	maxStreams     int
}

type apiEnvelope struct {
	Code      *int   `json:"code"`
	ServerID  string `json:"server"`
	ServiceID string `json:"service"`
}

type streamEnvelope struct {
	apiEnvelope
	Stream streamItem `json:"stream"`
}

type streamsEnvelope struct {
	apiEnvelope
	Streams []streamItem `json:"streams"`
}

type vhostsEnvelope struct {
	apiEnvelope
	Vhosts []vhostItem `json:"vhosts"`
}

type streamItem struct {
	ID      string `json:"id"`
	VhostID string `json:"vhost"`
	App     string `json:"app"`
	Name    string `json:"name"`
	Publish struct {
		Active bool            `json:"active"`
		CID    json.RawMessage `json:"cid"`
	} `json:"publish"`
	Video struct {
		Codec   string `json:"codec"`
		Profile string `json:"profile"`
		Level   string `json:"level"`
		Width   int    `json:"width"`
		Height  int    `json:"height"`
	} `json:"video"`
	Audio struct {
		Codec      string `json:"codec"`
		Profile    string `json:"profile"`
		SampleRate int    `json:"sample_rate"`
		Channels   int    `json:"channel"`
	} `json:"audio"`
}

type vhostItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func NewClient(cfg config.SRS) (*Client, error) {
	base, err := normalizeAPIBaseURL(cfg.HTTPAPIs)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(cfg.APIRequestTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	pageSize := cfg.SnapshotPageSize
	if pageSize <= 0 {
		pageSize = defaultStreamPageSize
	}
	if pageSize > hardMaxStreamPageSize {
		pageSize = hardMaxStreamPageSize
	}
	maxStreams := cfg.SnapshotMaxStreams
	if maxStreams <= 0 {
		maxStreams = defaultMaxStreams
	}
	if maxStreams > hardMaxStreams {
		maxStreams = hardMaxStreams
	}
	return &Client{
		baseURL: base, username: strings.TrimSpace(cfg.APIUsername), password: cfg.APIPassword,
		http: &http.Client{Timeout: timeout}, streamPageSize: pageSize, maxStreams: maxStreams,
	}, nil
}

// ListPublishers 一次性获取当前SRS中的全部发布流，并把运行时vhost ID转换为逻辑vhost名称。
// 调用方可以用这份有界分页快照批量处理场次，避免逐直播间请求SRS形成N+1流量。
func (c *Client) ListPublishers(ctx context.Context) ([]PublisherSnapshot, error) {
	vhostNames, err := c.loadVhostNames(ctx)
	if err != nil {
		return nil, err
	}
	streams, err := c.listStreams(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]PublisherSnapshot, 0, len(streams))
	for _, stream := range streams {
		vhost := strings.TrimSpace(stream.VhostID)
		if name, ok := vhostNames[vhost]; ok {
			vhost = name
		}
		result = append(result, PublisherSnapshot{
			StreamID: strings.TrimSpace(stream.ID), Vhost: vhost,
			App: strings.Trim(strings.TrimSpace(stream.App), "/"), Stream: strings.TrimSpace(stream.Name),
			Active:     stream.Publish.Active,
			VideoCodec: strings.TrimSpace(stream.Video.Codec), VideoProfile: strings.TrimSpace(stream.Video.Profile),
			VideoLevel: strings.TrimSpace(stream.Video.Level), Width: stream.Video.Width, Height: stream.Video.Height,
			AudioCodec: strings.TrimSpace(stream.Audio.Codec), AudioProfile: strings.TrimSpace(stream.Audio.Profile),
			AudioSampleRate: stream.Audio.SampleRate, AudioChannels: stream.Audio.Channels,
		})
	}
	return result, nil
}

// CurrentRuntime 通过轻量versions接口读取SRS进程身份。
func (c *Client) CurrentRuntime(ctx context.Context) (RuntimeIdentity, error) {
	var result apiEnvelope
	if err := c.doJSON(ctx, http.MethodGet, c.resourceURL("versions"), &result); err != nil {
		return RuntimeIdentity{}, err
	}
	identity := RuntimeIdentity{
		ServerID:  strings.TrimSpace(result.ServerID),
		ServiceID: strings.TrimSpace(result.ServiceID),
	}
	if identity.ServerID == "" {
		return RuntimeIdentity{}, errors.New("SRS HTTP API响应缺少server实例标识")
	}
	return identity, nil
}

// ListPublishersWithRuntime 在快照前后各校验一次server ID。
// 如果SRS恰好在分页期间重启，整份结果作废，避免把跨代际拼接的快照用于断流判断。
func (c *Client) ListPublishersWithRuntime(ctx context.Context) (PublisherBatch, error) {
	before, err := c.CurrentRuntime(ctx)
	if err != nil {
		return PublisherBatch{}, err
	}
	publishers, err := c.ListPublishers(ctx)
	if err != nil {
		return PublisherBatch{}, err
	}
	after, err := c.CurrentRuntime(ctx)
	if err != nil {
		return PublisherBatch{}, err
	}
	if before.ServerID != after.ServerID {
		return PublisherBatch{}, fmt.Errorf(
			"SRS在发布流快照期间重启: before=%s after=%s", before.ServerID, after.ServerID,
		)
	}
	return PublisherBatch{CapturedAt: time.Now().UnixMilli(), Runtime: after, Publishers: publishers}, nil
}

// StopPublisher 查询发布流对应的publish.cid，再通过DELETE clients/{cid}断开推流端。
// 流或客户端已经不存在等价于“已经停止”，因此按幂等成功处理。
func (c *Client) StopPublisher(ctx context.Context, ref StreamRef) error {
	stream, found, err := c.findStream(ctx, ref)
	if err != nil {
		return err
	}
	if !found || !stream.Publish.Active {
		return nil
	}
	cid, err := parseCID(stream.Publish.CID)
	if err != nil {
		return err
	}
	var result apiEnvelope
	err = c.doJSON(ctx, http.MethodDelete, c.resourceURL("clients", cid), &result)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// StopPublishers 只读取一次SRS流列表，再用固定worker删除匹配发布者。
// 相比逐场StopPublisher，它把1万场收尾的SRS查询从最多1万次压缩为一次分页快照；
// 每个DELETE仍保留独立HTTP超时，且不存在的流按幂等成功处理。
func (c *Client) StopPublishers(ctx context.Context, refs []StreamRef, concurrency int) []error {
	results := make([]error, len(refs))
	if len(refs) == 0 {
		return results
	}
	needsVhostNames := false
	for _, ref := range refs {
		if strings.TrimSpace(ref.Vhost) != "" {
			needsVhostNames = true
			break
		}
	}
	vhostNames := make(map[string]string)
	if needsVhostNames {
		var err error
		vhostNames, err = c.loadVhostNames(ctx)
		if err != nil {
			fillBatchStopErrors(results, err)
			return results
		}
	}
	streams, err := c.listStreams(ctx)
	if err != nil {
		fillBatchStopErrors(results, err)
		return results
	}

	byID := make(map[string]streamItem, len(streams))
	byAppAndName := make(map[string][]streamItem, len(streams))
	byName := make(map[string][]streamItem, len(streams))
	for _, stream := range streams {
		if id := strings.TrimSpace(stream.ID); id != "" {
			byID[id] = stream
		}
		key := streamLookupKey(stream.App, stream.Name)
		byAppAndName[key] = append(byAppAndName[key], stream)
		name := strings.TrimSpace(stream.Name)
		byName[name] = append(byName[name], stream)
	}

	type stopJob struct {
		index int
		cid   string
	}
	jobsToRun := make([]stopJob, 0, len(refs))
	for index, ref := range refs {
		stream, found := byID[strings.TrimSpace(ref.StreamID)]
		if !found || !streamMatches(stream, ref, vhostNames) {
			found = false
			candidates := byAppAndName[streamLookupKey(ref.App, ref.Stream)]
			if strings.Trim(strings.TrimSpace(ref.App), "/") == "" {
				candidates = byName[strings.TrimSpace(ref.Stream)]
			}
			if strings.TrimSpace(ref.Stream) == "" {
				candidates = streams
			}
			for _, candidate := range candidates {
				if streamMatches(candidate, ref, vhostNames) {
					stream, found = candidate, true
					break
				}
			}
		}
		if !found || !stream.Publish.Active {
			continue
		}
		cid, parseErr := parseCID(stream.Publish.CID)
		if parseErr != nil {
			results[index] = parseErr
			continue
		}
		jobsToRun = append(jobsToRun, stopJob{index: index, cid: cid})
	}
	if len(jobsToRun) == 0 {
		return results
	}
	if concurrency <= 0 {
		concurrency = 8
	}
	if concurrency > 64 {
		concurrency = 64
	}
	if concurrency > len(jobsToRun) {
		concurrency = len(jobsToRun)
	}
	jobs := make(chan stopJob, concurrency)
	var workers sync.WaitGroup
	for range concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				var response apiEnvelope
				deleteErr := c.doJSON(ctx, http.MethodDelete, c.resourceURL("clients", job.cid), &response)
				if errors.Is(deleteErr, ErrNotFound) {
					deleteErr = nil
				}
				results[job.index] = deleteErr
			}
		}()
	}
	for _, job := range jobsToRun {
		jobs <- job
	}
	close(jobs)
	workers.Wait()
	return results
}

func fillBatchStopErrors(results []error, err error) {
	for index := range results {
		results[index] = err
	}
}

func streamLookupKey(app, stream string) string {
	return strings.Trim(strings.TrimSpace(app), "/") + "\x00" + strings.TrimSpace(stream)
}

// findStream 在直播场次进入“结束中”后，从SRS中定位该场次当前的发布流，供StopPublisher断开推流连接。
// 调用场景包括：主播通过业务接口主动结束直播、后台管理员强制结束直播，以及断流超过重连窗口后的收尾确认。
// 主播仅关闭推流软件或网络断开时不会立即调用这里：SRS先通过on_unpublish通知业务进入重连窗口，
// 超过窗口仍未恢复后，定时任务才会调用这里确认流已不存在并完成场次结算。
// 定位时优先使用SRS stream ID；ID因SRS重启或重新推流而失效时，再按vhost/app/stream稳定标识查找。
func (c *Client) findStream(ctx context.Context, ref StreamRef) (streamItem, bool, error) {
	var vhostNames map[string]string
	if strings.TrimSpace(ref.Vhost) != "" {
		var err error
		vhostNames, err = c.loadVhostNames(ctx)
		if err != nil {
			return streamItem{}, false, err
		}
	}

	if id := strings.TrimSpace(ref.StreamID); id != "" {
		var result streamEnvelope
		if err := c.doJSON(ctx, http.MethodGet, c.resourceURL("streams", id), &result); err == nil {
			if streamMatches(result.Stream, ref, vhostNames) {
				return result.Stream, true, nil
			}
		} else if !errors.Is(err, ErrNotFound) {
			// 连接失败、认证失败或SRS业务错误必须直接返回，避免在同一次尝试中
			// 再发起列表请求并成倍放大故障流量。
			return streamItem{}, false, err
		}
		// SRS重启或重连后stream_id可能失效，继续按稳定的流三元组查找。
	}

	streams, err := c.listStreams(ctx)
	if err != nil {
		return streamItem{}, false, err
	}
	for _, candidate := range streams {
		if streamMatches(candidate, ref, vhostNames) {
			return candidate, true, nil
		}
	}
	return streamItem{}, false, nil
}

func (c *Client) listStreams(ctx context.Context) ([]streamItem, error) {
	streams := make([]streamItem, 0, min(c.streamPageSize, c.maxStreams))
	for len(streams) < c.maxStreams {
		count := min(c.streamPageSize, c.maxStreams-len(streams))
		endpoint, err := url.Parse(c.resourceURL("streams"))
		if err != nil {
			return nil, err
		}
		query := endpoint.Query()
		query.Set("start", strconv.Itoa(len(streams)))
		query.Set("count", strconv.Itoa(count))
		endpoint.RawQuery = query.Encode()

		var result streamsEnvelope
		if err = c.doJSON(ctx, http.MethodGet, endpoint.String(), &result); err != nil {
			return nil, err
		}
		if len(result.Streams) > count {
			return nil, fmt.Errorf("SRS发布流分页返回%d条，超过请求上限%d", len(result.Streams), count)
		}
		streams = append(streams, result.Streams...)
		if len(result.Streams) < count {
			return streams, nil
		}
	}

	// 达到上限后额外探测一条，区分“恰好等于上限”和“结果已被截断”。
	endpoint, err := url.Parse(c.resourceURL("streams"))
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("start", strconv.Itoa(len(streams)))
	query.Set("count", "1")
	endpoint.RawQuery = query.Encode()
	var result streamsEnvelope
	if err = c.doJSON(ctx, http.MethodGet, endpoint.String(), &result); err != nil {
		return nil, err
	}
	if len(result.Streams) > 0 {
		return nil, fmt.Errorf("SRS发布流数量超过配置上限%d，拒绝使用截断快照", c.maxStreams)
	}
	return streams, nil
}

// loadVhostNames 将SRS运行时生成的vhost资源ID解析为配置中的逻辑名称。
// /api/v1/streams返回的是类似vid-xxx的内部ID，而不是__defaultVhost__或域名；
// 内部ID会随SRS实例变化，不能持久化到直播session中。
func (c *Client) loadVhostNames(ctx context.Context) (map[string]string, error) {
	result := make(map[string]string)
	for page := 0; page < maxVhostPages; page++ {
		endpoint, err := url.Parse(c.resourceURL("vhosts"))
		if err != nil {
			return nil, err
		}
		query := endpoint.Query()
		query.Set("start", strconv.Itoa(page*vhostPageSize))
		query.Set("count", strconv.Itoa(vhostPageSize))
		endpoint.RawQuery = query.Encode()

		var response vhostsEnvelope
		if err = c.doJSON(ctx, http.MethodGet, endpoint.String(), &response); err != nil {
			return nil, err
		}
		for _, vhost := range response.Vhosts {
			id := strings.TrimSpace(vhost.ID)
			name := strings.TrimSpace(vhost.Name)
			if id != "" && name != "" {
				result[id] = name
			}
		}
		if len(response.Vhosts) < vhostPageSize {
			return result, nil
		}
	}
	return nil, fmt.Errorf("SRS vhost列表超过最大分页范围")
}

func streamMatches(stream streamItem, ref StreamRef, vhostNames map[string]string) bool {
	if expected := strings.TrimSpace(ref.Stream); expected != "" && stream.Name != expected {
		return false
	}
	if expected := strings.Trim(strings.TrimSpace(ref.App), "/"); expected != "" && stream.App != expected {
		return false
	}
	expectedVhost := strings.TrimSpace(ref.Vhost)
	actualVhost := strings.TrimSpace(stream.VhostID)
	if name, ok := vhostNames[actualVhost]; ok {
		actualVhost = name
	}
	if expectedVhost != "" && actualVhost == "" {
		return false
	}
	if expectedVhost != "" && !strings.EqualFold(expectedVhost, actualVhost) &&
		expectedVhost != "__defaultVhost__" && actualVhost != "__defaultVhost__" {
		return false
	}
	return true
}

func parseCID(raw json.RawMessage) (string, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return "", errors.New("SRS发布流缺少publish.cid")
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var decoded interface{}
	if err := decoder.Decode(&decoded); err != nil {
		return "", fmt.Errorf("SRS发布流publish.cid无效: %s", value)
	}
	var cid string
	switch typed := decoded.(type) {
	case string:
		cid = strings.TrimSpace(typed)
	case json.Number:
		cid = typed.String()
	default:
		return "", fmt.Errorf("SRS发布流publish.cid无效: %s", value)
	}
	if cid == "" {
		return "", fmt.Errorf("SRS发布流publish.cid无效: %s", value)
	}
	return cid, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, target interface{}) error {
	//return nil //codex 请不要修改，我调试用
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return err
	}
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("调用SRS HTTP API失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("读取SRS HTTP API响应失败: %w", err)
	}
	if len(body) > maxResponseBytes {
		return errors.New("SRS HTTP API响应过大")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("SRS HTTP API返回HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err = json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("解析SRS HTTP API响应失败: %w", err)
	}
	var envelope apiEnvelope
	if err = json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("解析SRS HTTP API状态失败: %w", err)
	}
	if envelope.Code == nil {
		return errors.New("SRS HTTP API响应缺少code")
	}
	if *envelope.Code != 0 {
		if *envelope.Code == srsCodeStreamNotFound || *envelope.Code == srsCodeClientNotFound {
			return fmt.Errorf("%w（SRS错误码%d）", ErrNotFound, *envelope.Code)
		}
		return fmt.Errorf("SRS HTTP API返回错误码%d", *envelope.Code)
	}
	return nil
}

func (c *Client) resourceURL(parts ...string) string {
	result := strings.TrimRight(c.baseURL, "/")
	for _, part := range parts {
		result += "/" + url.PathEscape(strings.TrimSpace(part))
	}
	return result
}

func normalizeAPIBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("live.srs.http_apis不能为空")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("live.srs.http_apis必须是有效的HTTP或HTTPS地址")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("live.srs.http_apis不能包含查询参数或片段")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(parsed.Path, "/api/v1") {
		parsed.Path += "/api/v1"
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
