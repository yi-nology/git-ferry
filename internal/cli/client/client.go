package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yi-nology/git-ferry/internal/cli/output"
)

// Client 是 GitFerry REST 的薄封装：注入 X-API-Key，把服务端响应映射为 Envelope。
type Client struct {
	BaseURL string
	APIKey  string
	Debug   bool
	HTTP    *http.Client
}

func New(baseURL, apiKey string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Do 发起请求并返回 Envelope。body/query 可为 nil。
func (c *Client) Do(method, path string, body interface{}, query url.Values) (*output.Envelope, error) {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		switch b := body.(type) {
		case []byte:
			reader = bytes.NewReader(b)
		case string:
			reader = strings.NewReader(b)
		case json.RawMessage:
			reader = bytes.NewReader(b)
		default:
			buf, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("marshal request body: %w", err)
			}
			reader = bytes.NewReader(buf)
		}
	}

	req, err := http.NewRequest(method, u, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.APIKey != "" {
		req.Header.Set("X-API-Key", c.APIKey)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return output.Fail(0, err.Error(), "检查 GITFERRY_BASE_URL 与服务是否启动（GET /health）"), nil
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if c.Debug {
		_, _ = fmt.Fprintf(io.Discard, "%s %s -> %d\n", method, path, resp.StatusCode)
	}

	return output.FromServer(resp.StatusCode, raw), nil
}

// Get / Post / Put / Delete 便捷方法。
func (c *Client) Get(path string, query url.Values) (*output.Envelope, error) {
	return c.Do(http.MethodGet, path, nil, query)
}
func (c *Client) Post(path string, body interface{}) (*output.Envelope, error) {
	return c.Do(http.MethodPost, path, body, nil)
}
func (c *Client) Put(path string, body interface{}) (*output.Envelope, error) {
	return c.Do(http.MethodPut, path, body, nil)
}
func (c *Client) Delete(path string, query url.Values) (*output.Envelope, error) {
	return c.Do(http.MethodDelete, path, nil, query)
}
