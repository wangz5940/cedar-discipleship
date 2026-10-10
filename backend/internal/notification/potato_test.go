package notification

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"syscall"
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
		{"chat assigned twice", "123:secret", `{"1":{"chat_id":99,"chat_type":2},"2":{"chat_id":99,"chat_type":2}}`, false},
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
		{"muted bot", 400, `{"ok":false,"error_code":3023}`, "potato_3023", false},
		{"HTTP body error code", 400, `{"ok":false,"error_code":1002,"result":"secret"}`, "potato_1002", false},
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

func TestPotatoListChats(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/getGroups" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{
			"ok":true,
			"result":{
				"Groups":[{"PeerID":10,"PeerName":"普通群"}],
				"SuperGroups":[{"PeerID":20,"PeerName":"2026 bible study"}],
				"Channels":[{"PeerID":30,"PeerName":"频道"}]
			}
		}`)
	}))
	defer server.Close()
	client, err := NewPotatoClient("123:secret")
	if err != nil {
		t.Fatal(err)
	}
	client.groupsEndpoint = server.URL + "/getGroups"
	chats, err := client.ListChats(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []Chat{
		{ChatID: 20, ChatType: 3, Title: "2026 bible study", Joined: true},
		{ChatID: 10, ChatType: 2, Title: "普通群", Joined: true},
	}
	if !reflect.DeepEqual(chats, want) {
		t.Fatalf("chats = %#v, want %#v", chats, want)
	}
}

func TestPotatoListChatsRetriesEmptySuccessResponse(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			return
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":{"Groups":[],"SuperGroups":[]}}`)
	}))
	defer server.Close()

	client, err := NewPotatoClient("123:secret")
	if err != nil {
		t.Fatal(err)
	}
	client.groupsEndpoint = server.URL + "/getGroups"
	if _, err := client.ListChats(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

func TestPotatoIdentity(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/getMe" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{
			"ok":true,
			"result":{"id":10100427,"first_name":" Primary Bot ","last_name":"","username":"primary_bot"}
		}`)
	}))
	defer server.Close()
	client, err := NewPotatoClient("123:secret")
	if err != nil {
		t.Fatal(err)
	}
	client.identityEndpoint = server.URL + "/getMe"
	identity, err := client.Identity(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := RobotIdentity{ID: 10100427, FirstName: "Primary Bot", Username: "primary_bot"}
	if identity != want {
		t.Fatalf("Identity() = %#v, want %#v", identity, want)
	}
}

func TestPotatoIdentityRetriesEmptySuccessResponse(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			return
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":{"id":10100427,"first_name":"Primary Bot","username":"primary_bot"}}`)
	}))
	defer server.Close()

	client, err := NewPotatoClient("123:secret")
	if err != nil {
		t.Fatal(err)
	}
	client.identityEndpoint = server.URL + "/getMe"
	if _, err := client.Identity(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

func TestPotatoIdentityDoesNotLeakTokenOrResponse(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "secret response", http.StatusUnauthorized)
	}))
	defer server.Close()
	client, err := NewPotatoClient("123:secret")
	if err != nil {
		t.Fatal(err)
	}
	client.identityEndpoint = server.URL + "/123:secret/getMe"
	err = func() error {
		_, err := client.Identity(t.Context())
		return err
	}()
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("Identity() error = %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}

func TestPotatoIdentityStopsAfterEmptySuccessResponses(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	client, err := NewPotatoClient("123:secret")
	if err != nil {
		t.Fatal(err)
	}
	client.identityEndpoint = server.URL + "/getMe"
	if _, err := client.Identity(t.Context()); err == nil {
		t.Fatal("Identity() error = nil")
	}
	if got := requests.Load(); got != potatoReadAttempts {
		t.Fatalf("requests = %d, want %d", got, potatoReadAttempts)
	}
}

type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

func TestPotatoTransportDiagnosticsNeverExposeToken(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		err  error
		want string
	}{
		{"canceled", context.Canceled, "canceled"},
		{"timeout", context.DeadlineExceeded, "timeout"},
		{"dns", &net.DNSError{Err: "lookup failed", Name: "private.example"}, "dns"},
		{"certificate", x509.UnknownAuthorityError{}, "tls_certificate"},
		{"refused", syscall.ECONNREFUSED, "connection_refused"},
		{"reset", syscall.ECONNRESET, "connection_reset"},
		{"closed", io.EOF, "connection_closed"},
		{"other", errors.New("123:secret unknown failure"), "network_other"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewPotatoClient("123:secret")
			if err != nil {
				t.Fatal(err)
			}
			client.client.Transport = failingTransport{err: &url.Error{Op: "Post", URL: client.endpoint, Err: tt.err}}
			err = client.SendText(t.Context(), Target{}, "test")
			var failure *deliveryError
			if !errors.As(err, &failure) || failure.Error() != "transport_failed" || !failure.retry || failure.reason != tt.want {
				t.Fatalf("failure=%+v", failure)
			}
			if strings.Contains(failure.Error()+failure.reason, "123:secret") {
				t.Fatal("token leaked")
			}
		})
	}
}
