package notification

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseTargets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, token, mapping string
		valid                bool
	}{
		{"disabled", "", "", true},
		{"group", "123:secret", `{"1":{"chat_id":99,"chat_type":2}}`, true},
		{"supergroup", "123:secret", `{"1":{"chat_id":99,"chat_type":3}}`, true},
		{"token missing", "", `{"1":{"chat_id":99,"chat_type":2}}`, false},
		{"mapping not assigned", "123:secret", "", true},
		{"empty mapping", "123:secret", `{}`, true},
		{"whitespace mapping", "123:secret", " \n ", true},
		{"null mapping", "123:secret", `null`, false},
		{"direct message forbidden", "123:secret", `{"1":{"chat_id":99,"chat_type":1}}`, false},
		{"zero group", "123:secret", `{"0":{"chat_id":99,"chat_type":2}}`, false},
		{"typo", "123:secret", `{"1":{"chat_id":99,"chat_type":2,"typo":1}}`, false},
		{"token path injection", "123:secret/../elsewhere", `{"1":{"chat_id":99,"chat_type":2}}`, false},
		{"trailing JSON", "123:secret", `{"1":{"chat_id":99,"chat_type":2}} {}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseTargets(tt.token, tt.mapping)
			if (err == nil) != tt.valid {
				t.Fatalf("ParseTargets() err=%v, valid=%v", err, tt.valid)
			}
		})
	}
}

func TestPotatoSendText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		body   string
		code   string
		retry  bool
	}{
		{"success", 200, `{"ok":true,"result":{"message_id":1}}`, "", false},
		{"invalid token", 200, `{"ok":false,"error_code":1002,"result":"secret"}`, "potato_1002", false},
		{"rate limit", 200, `{"ok":false,"error_code":1007}`, "potato_1007", true},
		{"slow mode", 200, `{"ok":false,"error_code":4048}`, "potato_4048", true},
		{"server error", 200, `{"ok":false,"error_code":1001}`, "potato_1001", true},
		{"forbidden", 403, "secret", "http_403", false},
		{"HTTP limit", 429, "", "http_429", true},
		{"upstream error", 503, "", "http_503", true},
		{"invalid body", 200, "<html>secret</html>", "response_invalid", true},
		{"missing ok", 200, `{}`, "potato_0", false},
		{"oversized", 200, strings.Repeat("x", 65537), "response_read_failed", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
					t.Error("invalid method or content type")
				}
				var request struct {
					Target
					Text     string `json:"text"`
					Markdown bool   `json:"markdown"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.Target != (Target{ChatID: 99, ChatType: 2}) || request.Text != "1 张三 【新】视频" || request.Markdown {
					t.Errorf("wrong payload: %#v", request)
				}
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(tt.status)
				if _, err := io.WriteString(w, tt.body); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client, err := NewPotatoClient("123:secret")
			if err != nil {
				t.Fatal(err)
			}
			client.endpoint = server.URL
			err = client.SendText(t.Context(), Target{ChatID: 99, ChatType: 2}, "1 张三 【新】视频")
			if tt.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var failure *deliveryError
			if !errors.As(err, &failure) || failure.code != tt.code || failure.retry != tt.retry {
				t.Fatalf("err = %v, want %s retry=%v", err, tt.code, tt.retry)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("token or response body leaked")
			}
			if tt.status == 429 && failure.retryAfter != time.Minute {
				t.Fatalf("retry delay = %v", failure.retryAfter)
			}
		})
	}
}

func TestPotatoRejectsRedirectsAndRedactsTransportError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/leak" {
			t.Error("redirect was followed")
		}
		http.Redirect(w, r, "/leak", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client, err := NewPotatoClient("123:secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(client.endpoint, "https://api.rct2008.com:8443/") || client.client.Timeout != 10*time.Second {
		t.Fatal("unsafe default client")
	}
	client.endpoint = server.URL + "/123:secret"
	if err := client.SendText(t.Context(), Target{}, "test"); err == nil || err.Error() != "http_307" {
		t.Fatalf("redirect err = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := client.SendText(ctx, Target{}, "test"); err == nil || err.Error() != "transport_failed" {
		t.Fatalf("transport err = %v", err)
	}
}
