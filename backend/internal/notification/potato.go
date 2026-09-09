package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Target struct {
	ChatID   int64 `json:"chat_id"`
	ChatType int   `json:"chat_type"`
}

var tokenPattern = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9_-]+$`)

func ParseTargets(token, value string) (map[uint64]Target, error) {
	value = strings.TrimSpace(value)
	if token == "" && value == "" {
		return nil, nil
	}
	if !tokenPattern.MatchString(token) {
		return nil, errors.New("invalid AGP_POTATO_BOT_TOKEN")
	}
	if value == "" {
		return nil, nil
	}
	var raw map[string]Target
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil || raw == nil {
		return nil, errors.New("AGP_POTATO_GROUPS must be a group ID to chat mapping")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid AGP_POTATO_GROUPS JSON")
	}
	targets := make(map[uint64]Target, len(raw))
	for key, target := range raw {
		id, err := strconv.ParseUint(key, 10, 64)
		if err != nil || id == 0 || strconv.FormatUint(id, 10) != key ||
			target.ChatID <= 0 || (target.ChatType != 2 && target.ChatType != 3) {
			return nil, errors.New("AGP_POTATO_GROUPS requires positive IDs and group chat_type 2 or 3")
		}
		targets[id] = target
	}
	return targets, nil
}

type deliveryError struct {
	code       string
	retry      bool
	retryAfter time.Duration
}

func (e *deliveryError) Error() string { return e.code }

type PotatoClient struct {
	endpoint string
	client   *http.Client
}

func NewPotatoClient(token string) (*PotatoClient, error) {
	if !tokenPattern.MatchString(token) {
		return nil, errors.New("invalid AGP_POTATO_BOT_TOKEN")
	}
	return &PotatoClient{
		endpoint: "https://api.rct2008.com:8443/" + token + "/sendTextMessage",
		client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *PotatoClient) SendText(ctx context.Context, target Target, text string) error {
	body, err := json.Marshal(struct {
		Target
		Text     string `json:"text"`
		Markdown bool   `json:"markdown"`
	}{Target: target, Text: text})
	if err != nil {
		return &deliveryError{code: "encode_failed"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return &deliveryError{code: "request_invalid"}
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := c.client.Do(req)
	if err != nil {
		// net/http errors include the token-bearing URL. Never return them to logs.
		return &deliveryError{code: "transport_failed", retry: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &deliveryError{
			code:       fmt.Sprintf("http_%d", resp.StatusCode),
			retry:      resp.StatusCode == 429 || resp.StatusCode >= 500,
			retryAfter: retryAfter(resp.Header.Get("Retry-After"), time.Now()),
		}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
	if err != nil || len(data) > 64*1024 {
		return &deliveryError{code: "response_read_failed", retry: true}
	}
	var result struct {
		OK        bool `json:"ok"`
		ErrorCode int  `json:"error_code"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return &deliveryError{code: "response_invalid", retry: true}
	}
	if !result.OK {
		return &deliveryError{
			code: fmt.Sprintf("potato_%d", result.ErrorCode),
			retry: result.ErrorCode == 1001 || result.ErrorCode == 1007 ||
				result.ErrorCode == 4048,
		}
	}
	return nil
}

func retryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date.Sub(now)
	}
	return 0
}
