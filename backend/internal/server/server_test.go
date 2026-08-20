package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	assetdomain "agp/backend/internal/asset"
	statisticsdomain "agp/backend/internal/statistics"
)

func TestSignTokenPermanentByDefault(t *testing.T) {
	a := &app{secret: []byte("test-secret")}

	token, err := a.signToken(tokenClaims{UserID: 1})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	claims, err := a.verifyToken(token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if claims.ExpiresAt != 0 {
		t.Fatalf("token expiration = %d, want permanent token without exp", claims.ExpiresAt)
	}
}

func TestSignTokenAddsConfiguredExpiration(t *testing.T) {
	a := &app{
		secret:   []byte("test-secret"),
		tokenTTL: time.Hour,
	}

	token, err := a.signToken(tokenClaims{UserID: 1})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	claims, err := a.verifyToken(token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if claims.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("token expiration = %d, want a future timestamp", claims.ExpiresAt)
	}
}

func TestVerifyTokenRejectsExpiredToken(t *testing.T) {
	a := &app{secret: []byte("test-secret")}

	token, err := a.signToken(tokenClaims{
		UserID:    1,
		ExpiresAt: time.Now().Add(-time.Second).Unix(),
	})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	if _, err := a.verifyToken(token); err == nil {
		t.Fatal("verify token succeeded for an expired token")
	}
}

func TestLoginLimiterSupportsConcurrentRequests(t *testing.T) {
	limiter := newLoginLimiter()
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			limiter.fail("127.0.0.1", "member")
			_ = limiter.blocked("127.0.0.1", "member")
		}()
	}
	wg.Wait()
	if !limiter.blocked("127.0.0.1", "member") {
		t.Fatal("limiter should block after repeated failed attempts")
	}
	limiter.success("127.0.0.1", "member")
	if limiter.blocked("127.0.0.1", "member") {
		t.Fatal("limiter should clear after a successful login")
	}
}

func TestLoginLimiterBoundsFailureEntries(t *testing.T) {
	limiter := newLoginLimiter()
	for index := range maxLoginFailureEntries + 100 {
		limiter.fail("127.0.0.1", strconv.Itoa(index))
	}
	if len(limiter.failures) > maxLoginFailureEntries {
		t.Fatalf("failure entries = %d, want at most %d", len(limiter.failures), maxLoginFailureEntries)
	}
}

func TestClientIPUsesLastForwardedAddress(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("X-Forwarded-For", "203.0.113.10, 192.0.2.20")
	if got := clientIP(request); got != "192.0.2.20" {
		t.Fatalf("clientIP() = %q, want trusted proxy address", got)
	}
}

func TestReadJSONWithLimitRejectsOversizedPayload(t *testing.T) {
	request := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"value":"too large"}`))
	recorder := httptest.NewRecorder()
	var payload map[string]any
	if readJSONWithLimit(recorder, request, &payload, 8) {
		t.Fatal("readJSONWithLimit accepted oversized input")
	}
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestWriteErrorSetsErrorCodeHeader(t *testing.T) {
	recorder := httptest.NewRecorder()

	writeError(recorder, http.StatusUnauthorized, "unauthorized")

	if got := recorder.Header().Get("X-AGP-Error-Code"); got != "unauthorized" {
		t.Fatalf("error code header = %q, want unauthorized", got)
	}
}

func TestWithRequestLoggingRecordsErrorResponses(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	defer slog.SetDefault(previous)

	handler := withRequestLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/assets/16/download", nil))

	line := output.String()
	for _, want := range []string{
		"msg=\"http request rejected\"",
		"method=GET",
		"path=/api/assets/16/download",
		"status=401",
		"error_code=unauthorized",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("log output %q does not contain %q", line, want)
		}
	}
}

func TestSafeCSVCellEscapesFormulaPrefix(t *testing.T) {
	for _, value := range []string{"=cmd()", "+1", "-1", "@SUM(A1:A2)", " \t=cmd()"} {
		if got := safeCSVCell(value); !strings.HasPrefix(got, "'") {
			t.Fatalf("safeCSVCell(%q) = %q, want apostrophe prefix", value, got)
		}
	}
	if got := safeCSVCell("ordinary"); got != "ordinary" {
		t.Fatalf("safeCSVCell(ordinary) = %q", got)
	}
}

func TestValidateConfigRejectsWeakSecrets(t *testing.T) {
	if err := validateConfig(config{
		JWTSecret:         "short",
		BootstrapPassword: "StrongPass123",
	}); err == nil {
		t.Fatal("validateConfig accepted a short JWT secret")
	}
	if err := validateConfig(config{
		JWTSecret:         "12345678901234567890123456789012",
		BootstrapPassword: "short",
	}); err == nil {
		t.Fatal("validateConfig accepted a short bootstrap password")
	}
	if err := validateConfig(config{
		JWTSecret:         "12345678901234567890123456789012",
		BootstrapPassword: "StrongPass123",
		TokenTTL:          "bad",
	}); err == nil {
		t.Fatal("validateConfig accepted an invalid token TTL")
	}
	if err := validateConfig(config{
		JWTSecret:         "12345678901234567890123456789012",
		BootstrapPassword: "StrongPass123",
		TokenTTL:          "24h",
	}); err != nil {
		t.Fatalf("validateConfig returned error: %v", err)
	}
}

func TestValidCheckinTaskType(t *testing.T) {
	for _, taskType := range []string{"daily_devotion", "weekly_book", "weekly_video", "weekly_verse", "weekly_outline"} {
		if !validCheckinTaskType(taskType) {
			t.Fatalf("validCheckinTaskType(%q) = false", taskType)
		}
	}
	if validCheckinTaskType("reflection") {
		t.Fatal("validCheckinTaskType accepted an unsupported task type")
	}
}

func TestNormalizeActiveMemberRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     statisticsdomain.ActiveMemberRuleVO
		wantValid bool
		wantMode  string
		wantTypes []string
	}{
		{
			name:      "union keeps configured task order",
			input:     statisticsdomain.ActiveMemberRuleVO{Mode: "ANY", TaskTypes: []string{"weekly_video", "daily_devotion"}},
			wantValid: true,
			wantMode:  "any",
			wantTypes: []string{"daily_devotion", "weekly_video"},
		},
		{
			name:      "intersection removes duplicate tasks",
			input:     statisticsdomain.ActiveMemberRuleVO{Mode: "all", TaskTypes: []string{"weekly_book", "weekly_book"}},
			wantValid: true,
			wantMode:  "all",
			wantTypes: []string{"weekly_book"},
		},
		{
			name:  "empty selection rejected",
			input: statisticsdomain.ActiveMemberRuleVO{Mode: "any"},
		},
		{
			name:  "unknown task rejected",
			input: statisticsdomain.ActiveMemberRuleVO{Mode: "any", TaskTypes: []string{"weekly_verse"}},
		},
		{
			name:  "unknown mode rejected",
			input: statisticsdomain.ActiveMemberRuleVO{Mode: "none", TaskTypes: []string{"daily_devotion"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, valid := normalizeActiveMemberRule(test.input)
			if valid != test.wantValid {
				t.Fatalf("normalizeActiveMemberRule() valid = %v, want %v", valid, test.wantValid)
			}
			if !test.wantValid {
				return
			}
			if got.Mode != test.wantMode {
				t.Fatalf("mode = %q, want %q", got.Mode, test.wantMode)
			}
			if strings.Join(got.TaskTypes, ",") != strings.Join(test.wantTypes, ",") {
				t.Fatalf("task types = %v, want %v", got.TaskTypes, test.wantTypes)
			}
		})
	}
}

func TestWeeklyVerseTaskTitle(t *testing.T) {
	tests := []struct {
		name          string
		req           studyWeekInput
		existingTitle string
		want          string
	}{
		{
			name: "disabled returns empty",
			req: studyWeekInput{
				VerseEnabled: false,
			},
			existingTitle: "背经任务1",
			want:          "",
		},
		{
			name: "prefers verse ref",
			req: studyWeekInput{
				VerseEnabled: true,
				VerseRef:     "约翰福音 3:16",
			},
			existingTitle: "背经任务1",
			want:          "约翰福音 3:16",
		},
		{
			name: "falls back to existing title",
			req: studyWeekInput{
				VerseEnabled: true,
			},
			existingTitle: "背经任务1",
			want:          "背经任务1",
		},
		{
			name: "uses default title when empty",
			req: studyWeekInput{
				VerseEnabled: true,
			},
			existingTitle: "",
			want:          "背经",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weeklyVerseTaskTitle(tt.req, tt.existingTitle)
			if got != tt.want {
				t.Fatalf("weeklyVerseTaskTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveExistingFileInRootsFallsBackToContentRoot(t *testing.T) {
	tempDir := t.TempDir()
	assetsRoot := filepath.Join(tempDir, "assets")
	contentRoot := filepath.Join(tempDir, "content")
	if err := os.MkdirAll(filepath.Join(contentRoot, "Book"), 0o755); err != nil {
		t.Fatalf("mkdir content root: %v", err)
	}
	want := filepath.Join(contentRoot, "Book", "sample.pdf")
	if err := os.WriteFile(want, []byte("pdf"), 0o644); err != nil {
		t.Fatalf("write content file: %v", err)
	}

	got, original, err := assetdomain.ResolveExistingFileInRoots("/Book/sample.pdf", assetsRoot, contentRoot)
	if err != nil {
		t.Fatalf("resolveExistingFileInRoots returned error: %v", err)
	}
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if original != "sample.pdf" {
		t.Fatalf("expected original sample.pdf, got %q", original)
	}
}

func TestResolveExistingFileInRootsPrefersAssetsRoot(t *testing.T) {
	tempDir := t.TempDir()
	assetsRoot := filepath.Join(tempDir, "assets")
	contentRoot := filepath.Join(tempDir, "content")
	if err := os.MkdirAll(filepath.Join(assetsRoot, "shared"), 0o755); err != nil {
		t.Fatalf("mkdir assets root: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(contentRoot, "shared"), 0o755); err != nil {
		t.Fatalf("mkdir content root: %v", err)
	}
	want := filepath.Join(assetsRoot, "shared", "sample.pdf")
	if err := os.WriteFile(want, []byte("asset"), 0o644); err != nil {
		t.Fatalf("write assets file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentRoot, "shared", "sample.pdf"), []byte("content"), 0o644); err != nil {
		t.Fatalf("write content file: %v", err)
	}

	got, _, err := assetdomain.ResolveExistingFileInRoots("shared/sample.pdf", assetsRoot, contentRoot)
	if err != nil {
		t.Fatalf("resolveExistingFileInRoots returned error: %v", err)
	}
	if got != want {
		t.Fatalf("expected assets root path %q, got %q", want, got)
	}
}

func TestResolveExistingFileInRootsReturnsErrorWhenMissing(t *testing.T) {
	tempDir := t.TempDir()
	assetsRoot := filepath.Join(tempDir, "assets")
	contentRoot := filepath.Join(tempDir, "content")

	if _, _, err := assetdomain.ResolveExistingFileInRoots("/Book/missing.pdf", assetsRoot, contentRoot); err == nil {
		t.Fatal("expected missing file error")
	}
}

func TestStaticAssetDownloadPath(t *testing.T) {
	path, ok := staticAssetDownloadPath("book:", "Book/%E5%9F%BA%E7%9D%A3.pdf/download")
	if !ok {
		t.Fatal("staticAssetDownloadPath rejected a valid static asset link")
	}
	if path != "/Book/%E5%9F%BA%E7%9D%A3.pdf" {
		t.Fatalf("path = %q", path)
	}
	if _, ok := staticAssetDownloadPath("unknown:", "Book/a.pdf/download"); ok {
		t.Fatal("staticAssetDownloadPath accepted an unknown prefix")
	}
	if _, ok := staticAssetDownloadPath("book:", "Book/a.pdf"); ok {
		t.Fatal("staticAssetDownloadPath accepted a path without /download")
	}
}
