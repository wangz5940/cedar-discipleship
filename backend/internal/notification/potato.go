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
	"sort"
	"strconv"
	"strings"
	"time"
)

type Target struct {
	ChatID   int64 `json:"chat_id"`
	ChatType int   `json:"chat_type"`
}

var tokenPattern = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9_-]+$`)

const potatoReadAttempts = 3

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
	chatIDs := make(map[int64]struct{}, len(raw))
	for key, target := range raw {
		id, err := strconv.ParseUint(key, 10, 64)
		if err != nil || id == 0 || strconv.FormatUint(id, 10) != key ||
			target.ChatID <= 0 || (target.ChatType != 2 && target.ChatType != 3) {
			return nil, errors.New("AGP_POTATO_GROUPS requires positive IDs and group chat_type 2 or 3")
		}
		if _, exists := chatIDs[target.ChatID]; exists {
			return nil, errors.New("AGP_POTATO_GROUPS assigns a chat more than once")
		}
		chatIDs[target.ChatID] = struct{}{}
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

type Chat struct {
	ChatID         int64  `json:"chat_id"`
	ChatType       int    `json:"chat_type"`
	Title          string `json:"title"`
	GroupID        uint64 `json:"group_id,omitempty"`
	BoundElsewhere bool   `json:"bound_elsewhere,omitempty"`
	Joined         bool   `json:"joined"`
}

type RobotIdentity struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type PotatoClient struct {
	endpoint         string
	groupsEndpoint   string
	identityEndpoint string
	client           *http.Client
}

var newPotatoClient = newPotatoClientWithToken

func NewPotatoClient(token string) (*PotatoClient, error) {
	return newPotatoClient(token)
}

func newPotatoClientWithToken(token string) (*PotatoClient, error) {
	if !tokenPattern.MatchString(token) {
		return nil, errors.New("invalid AGP_POTATO_BOT_TOKEN")
	}
	return &PotatoClient{
		endpoint:         "https://api.rct2008.com:8443/" + token + "/sendTextMessage",
		groupsEndpoint:   "https://api.rct2008.com:8443/" + token + "/getGroups",
		identityEndpoint: "https://api.rct2008.com:8443/" + token + "/getMe",
		client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *PotatoClient) Identity(ctx context.Context) (RobotIdentity, error) {
	var data []byte
	for attempt := 0; attempt < potatoReadAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.identityEndpoint, nil)
		if err != nil {
			return RobotIdentity{}, errors.New("create robot identity request")
		}
		resp, err := c.client.Do(req)
		if err != nil {
			return RobotIdentity{}, errors.New("request robot identity")
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return RobotIdentity{}, fmt.Errorf("robot identity http_%d", resp.StatusCode)
		}
		if err != nil || len(data) > 64*1024 {
			return RobotIdentity{}, errors.New("read robot identity response")
		}
		if len(data) > 0 {
			break
		}
	}
	var result struct {
		OK     bool          `json:"ok"`
		Result RobotIdentity `json:"result"`
	}
	if err := json.Unmarshal(data, &result); err != nil || !result.OK ||
		result.Result.ID <= 0 || strings.TrimSpace(result.Result.Username) == "" {
		return RobotIdentity{}, errors.New("invalid robot identity response")
	}
	result.Result.FirstName = strings.TrimSpace(result.Result.FirstName)
	result.Result.Username = strings.TrimSpace(result.Result.Username)
	return result.Result, nil
}

func (c *PotatoClient) ListChats(ctx context.Context) ([]Chat, error) {
	var data []byte
	for attempt := 0; attempt < potatoReadAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.groupsEndpoint, nil)
		if err != nil {
			return nil, errors.New("create group list request")
		}
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, errors.New("request group list")
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("group list http_%d", resp.StatusCode)
		}
		if err != nil || len(data) > 64*1024 {
			return nil, errors.New("read group list response")
		}
		if len(data) > 0 {
			break
		}
	}
	type group struct {
		PeerID   int64  `json:"PeerID"`
		PeerName string `json:"PeerName"`
	}
	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			Groups      []group `json:"Groups"`
			SuperGroups []group `json:"SuperGroups"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &result); err != nil || !result.OK {
		return nil, errors.New("invalid group list response")
	}
	chats := make([]Chat, 0, len(result.Result.Groups)+len(result.Result.SuperGroups))
	seen := make(map[int64]struct{})
	appendGroups := func(groups []group, chatType int) error {
		for _, item := range groups {
			if item.PeerID <= 0 || strings.TrimSpace(item.PeerName) == "" {
				return errors.New("invalid group list item")
			}
			if _, exists := seen[item.PeerID]; exists {
				return errors.New("duplicate group list item")
			}
			seen[item.PeerID] = struct{}{}
			chats = append(chats, Chat{
				ChatID: item.PeerID, ChatType: chatType, Title: strings.TrimSpace(item.PeerName), Joined: true,
			})
		}
		return nil
	}
	if err := appendGroups(result.Result.Groups, 2); err != nil {
		return nil, err
	}
	if err := appendGroups(result.Result.SuperGroups, 3); err != nil {
		return nil, err
	}
	sort.Slice(chats, func(i, j int) bool {
		if chats[i].Title != chats[j].Title {
			return chats[i].Title < chats[j].Title
		}
		return chats[i].ChatID < chats[j].ChatID
	})
	return chats, nil
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
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if readErr == nil && len(data) <= 64*1024 {
			var result struct {
				OK        bool `json:"ok"`
				ErrorCode int  `json:"error_code"`
			}
			if json.Unmarshal(data, &result) == nil && !result.OK && result.ErrorCode != 0 {
				return potatoDeliveryError(result.ErrorCode)
			}
		}
		return &deliveryError{
			code:       fmt.Sprintf("http_%d", resp.StatusCode),
			retry:      resp.StatusCode == 429 || resp.StatusCode >= 500,
			retryAfter: retryAfter(resp.Header.Get("Retry-After"), time.Now()),
		}
	}
	if readErr != nil || len(data) > 64*1024 {
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
		return potatoDeliveryError(result.ErrorCode)
	}
	return nil
}

func potatoDeliveryError(code int) *deliveryError {
	return &deliveryError{
		code:  fmt.Sprintf("potato_%d", code),
		retry: code == 1001 || code == 1007 || code == 4048,
	}
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
