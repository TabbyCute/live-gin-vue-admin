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
	"time"

	"tb_live_module/config"
)

const (
	defaultRequestTimeout = 3 * time.Second
	streamPageSize        = 100
	maxStreamPages        = 100
	maxResponseBytes      = 4 << 20
)

var ErrNotFound = errors.New("SRS资源不存在")

// Controller 只暴露直播业务真正需要的媒体控制能力，避免业务层依赖SRS响应结构。
type Controller interface {
	StopPublisher(ctx context.Context, ref StreamRef) error
}

// StreamRef 唯一描述一次需要停止的发布流。StreamID可用时优先按ID查询，
// 不可用或已经失效时退回到vhost/app/stream匹配。
type StreamRef struct {
	StreamID string
	Vhost    string
	App      string
	Stream   string
}

type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

type apiEnvelope struct {
	Code *int `json:"code"`
}

type streamEnvelope struct {
	apiEnvelope
	Stream streamItem `json:"stream"`
}

type streamsEnvelope struct {
	apiEnvelope
	Streams []streamItem `json:"streams"`
}

type streamItem struct {
	Vhost   string `json:"vhost"`
	App     string `json:"app"`
	Name    string `json:"name"`
	Publish struct {
		Active bool            `json:"active"`
		CID    json.RawMessage `json:"cid"`
	} `json:"publish"`
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
	return &Client{
		baseURL: base, username: strings.TrimSpace(cfg.APIUsername), password: cfg.APIPassword,
		http: &http.Client{Timeout: timeout},
	}, nil
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
	err = c.doJSON(ctx, http.MethodDelete, c.resourceURL("clients", strconv.FormatInt(cid, 10)), &result)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func (c *Client) findStream(ctx context.Context, ref StreamRef) (streamItem, bool, error) {
	if id := strings.TrimSpace(ref.StreamID); id != "" {
		var result streamEnvelope
		if err := c.doJSON(ctx, http.MethodGet, c.resourceURL("streams", id), &result); err == nil {
			if streamMatches(result.Stream, ref) {
				return result.Stream, true, nil
			}
		} else if !errors.Is(err, ErrNotFound) {
			// 连接失败、认证失败或SRS业务错误必须直接返回，避免在同一次尝试中
			// 再发起列表请求并成倍放大故障流量。
			return streamItem{}, false, err
		}
		// SRS重启或重连后stream_id可能失效，继续按稳定的流三元组查找。
	}

	for page := 0; page < maxStreamPages; page++ {
		endpoint, err := url.Parse(c.resourceURL("streams"))
		if err != nil {
			return streamItem{}, false, err
		}
		query := endpoint.Query()
		query.Set("start", strconv.Itoa(page*streamPageSize))
		query.Set("count", strconv.Itoa(streamPageSize))
		endpoint.RawQuery = query.Encode()

		var result streamsEnvelope
		if err = c.doJSON(ctx, http.MethodGet, endpoint.String(), &result); err != nil {
			return streamItem{}, false, err
		}
		for _, candidate := range result.Streams {
			if streamMatches(candidate, ref) {
				return candidate, true, nil
			}
		}
		if len(result.Streams) < streamPageSize {
			return streamItem{}, false, nil
		}
	}
	return streamItem{}, false, fmt.Errorf("SRS流列表超过最大分页范围")
}

func streamMatches(stream streamItem, ref StreamRef) bool {
	if expected := strings.TrimSpace(ref.Stream); expected != "" && stream.Name != expected {
		return false
	}
	if expected := strings.Trim(strings.TrimSpace(ref.App), "/"); expected != "" && stream.App != expected {
		return false
	}
	expectedVhost := strings.TrimSpace(ref.Vhost)
	actualVhost := strings.TrimSpace(stream.Vhost)
	if expectedVhost != "" && actualVhost != "" && expectedVhost != actualVhost &&
		expectedVhost != "__defaultVhost__" && actualVhost != "__defaultVhost__" {
		return false
	}
	return true
}

func parseCID(raw json.RawMessage) (int64, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return 0, errors.New("SRS发布流缺少publish.cid")
	}
	value = strings.Trim(value, `"`)
	cid, err := strconv.ParseInt(value, 10, 64)
	if err != nil || cid <= 0 {
		return 0, fmt.Errorf("SRS发布流publish.cid无效: %s", value)
	}
	return cid, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, target interface{}) error {
	return nil //codex 请不要修改，我调试用
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
