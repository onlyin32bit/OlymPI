package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/config"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
	"github.com/onlyin32bit/4t/internal/settings"
	"github.com/onlyin32bit/4t/internal/sound"
	store "github.com/onlyin32bit/4t/internal/store/sqlite"
)

func TestAuthRoutes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	sessions := auth.NewService(database)
	if _, err := sessions.CreateAdmin(ctx, "admin", "password123"); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.EnsureFixedContestants(ctx); err != nil {
		t.Fatal(err)
	}
	fixed, err := sessions.ListFixedContestants(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var cfg config.Config
	cfg.Server.Port = 8090
	cfg.Server.ServeUI = false
	cfg.Server.SecureCookies = true
	socket := opsocket.NewSocket()
	siteSettings, err := settings.Open(filepath.Join(t.TempDir(), "settings.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	games, err := game.NewService(ctx, database, fixed, nil)
	if err != nil {
		t.Fatal(err)
	}
	sounds, err := sound.NewService(ctx, database, siteSettings, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := New(cfg, sessions, siteSettings, games, sounds, socket)

	form := url.Values{"username": {"admin"}, "password": {"password123"}}
	req := mustRequest(t, http.MethodPost, "/api/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := server.app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("login returned %d: %s", response.StatusCode, body)
	}
	if len(response.Cookies()) != 1 {
		t.Fatal("login did not set the session cookie")
	}
	if !response.Cookies()[0].Secure {
		t.Fatal("configured secure cookies were not marked Secure")
	}

	me := mustRequest(t, http.MethodGet, "/api/me", nil)
	me.AddCookie(response.Cookies()[0])
	meResponse, err := server.app.Test(me)
	if err != nil {
		t.Fatal(err)
	}
	if meResponse.StatusCode != http.StatusOK {
		t.Fatalf("me returned %d", meResponse.StatusCode)
	}

	settingsResponse, err := server.app.Test(mustRequest(t, http.MethodGet, "/api/settings", nil))
	if err != nil || settingsResponse.StatusCode != http.StatusOK {
		t.Fatalf("public settings returned %d: %v", settingsResponse.StatusCode, err)
	}

	update := settings.Values{
		EventName: "4T Updated",
		HostName:  "Demo host",
		Sound:     siteSettings.Get().Sound,
	}
	body, _ := json.Marshal(update)
	unauthorized := mustRequest(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	unauthorized.Header.Set("Content-Type", "application/json")
	unauthorizedResponse, err := server.app.Test(unauthorized)
	if err != nil || unauthorizedResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized update returned %d: %v", unauthorizedResponse.StatusCode, err)
	}

	foreignOrigin := mustRequest(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	foreignOrigin.Host = "localhost:8090"
	foreignOrigin.Header.Set("Content-Type", "application/json")
	foreignOrigin.Header.Set("Origin", "https://evil.example")
	foreignOrigin.AddCookie(response.Cookies()[0])
	foreignResponse, err := server.app.Test(foreignOrigin)
	if err != nil || foreignResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign-origin update returned %d: %v", foreignResponse.StatusCode, err)
	}

	authorized := mustRequest(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	authorized.Host = "localhost:8090"
	authorized.Header.Set("Content-Type", "application/json")
	authorized.Header.Set("Origin", "http://localhost:5173")
	authorized.AddCookie(response.Cookies()[0])
	authorizedResponse, err := server.app.Test(authorized)
	if err != nil || authorizedResponse.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(authorizedResponse.Body)
		t.Fatalf("settings update returned %d: %s (%v)", authorizedResponse.StatusCode, responseBody, err)
	}
	want := update
	want.Scoring = settings.DefaultScoring()
	if got := siteSettings.Get(); got != want {
		t.Fatalf("settings were not updated, or omitted scoring was not preserved: %+v", got)
	}

	scored := want
	scored.Scoring.VeDich.Tiers = [2]int{20, 40}
	scoredBody, _ := json.Marshal(scored)
	scoredRequest := mustRequest(t, http.MethodPut, "/api/settings", bytes.NewReader(scoredBody))
	scoredRequest.Host = "localhost:8090"
	scoredRequest.Header.Set("Content-Type", "application/json")
	scoredRequest.Header.Set("Origin", "http://localhost:5173")
	scoredRequest.AddCookie(response.Cookies()[0])
	scoredResponse, err := server.app.Test(scoredRequest)
	if err != nil || scoredResponse.StatusCode != http.StatusOK {
		t.Fatalf("scoring update returned %d: %v", scoredResponse.StatusCode, err)
	}
	if got := siteSettings.Get().Scoring.VeDich.Tiers; got != [2]int{20, 40} {
		t.Fatalf("scoring update was not applied: %v", got)
	}

	questionPayload := map[string]any{
		"name": "Đề kiểm thử",
		"data": map[string]any{
			"kd_chung": []any{}, "kd_rieng": []any{[]any{}, []any{}, []any{}, []any{}},
			"vcnv": map[string]any{"obstacle": "A", "obstacle_asset_id": "asset", "rows": []any{map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{}}, "center": map[string]any{}},
			"tt":   []any{}, "vd": []any{},
		},
	}
	questionBody, _ := json.Marshal(questionPayload)
	createQuestion := mustRequest(t, http.MethodPost, "/api/question-sets", bytes.NewReader(questionBody))
	createQuestion.Header.Set("Content-Type", "application/json")
	createQuestion.AddCookie(response.Cookies()[0])
	questionResponse, err := server.app.Test(createQuestion, -1)
	if err != nil || questionResponse.StatusCode != http.StatusCreated {
		responseBody, _ := io.ReadAll(questionResponse.Body)
		t.Fatalf("question creation returned %d: %s (%v)", questionResponse.StatusCode, responseBody, err)
	}
	var createdQuestion struct {
		QuestionSet game.QuestionSet `json:"question_set"`
	}
	if err := json.NewDecoder(questionResponse.Body).Decode(&createdQuestion); err != nil {
		t.Fatal(err)
	}

	contestants := make([]game.Contestant, 4)
	for i := range contestants {
		contestants[i] = game.Contestant{Slot: i + 1, Name: "Thí sinh " + string(rune('1'+i)), Class: "10A"}
	}
	matchBody, _ := json.Marshal(game.CreateMatchInput{Name: "Trận kiểm thử", Season: 1, Stage: "Tứ kết", QuestionSetID: createdQuestion.QuestionSet.ID, Contestants: contestants})
	createMatch := mustRequest(t, http.MethodPost, "/api/matches", bytes.NewReader(matchBody))
	createMatch.Header.Set("Content-Type", "application/json")
	createMatch.AddCookie(response.Cookies()[0])
	matchResponse, err := server.app.Test(createMatch, -1)
	if err != nil || matchResponse.StatusCode != http.StatusCreated {
		responseBody, _ := io.ReadAll(matchResponse.Body)
		t.Fatalf("match creation returned %d: %s (%v)", matchResponse.StatusCode, responseBody, err)
	}

	for _, target := range []string{"/api/question-sets", "/api/matches", "/api/contestant-accounts", "/api/audit-logs"} {
		request := mustRequest(t, http.MethodGet, target, nil)
		request.AddCookie(response.Cookies()[0])
		listResponse, listErr := server.app.Test(request, -1)
		if listErr != nil || listResponse.StatusCode != http.StatusOK {
			t.Fatalf("%s returned %d: %v", target, listResponse.StatusCode, listErr)
		}
	}
}

func TestRequestLogLevel(t *testing.T) {
	tests := []struct {
		status int
		want   slog.Level
	}{
		{status: http.StatusOK, want: slog.LevelDebug},
		{status: http.StatusFound, want: slog.LevelInfo},
		{status: http.StatusBadRequest, want: slog.LevelWarn},
		{status: http.StatusUnauthorized, want: slog.LevelWarn},
		{status: http.StatusInternalServerError, want: slog.LevelError},
	}
	for _, test := range tests {
		if got := requestLogLevel(test.status); got != test.want {
			t.Errorf("requestLogLevel(%d) = %v, want %v", test.status, got, test.want)
		}
	}
}

func mustRequest(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	if req.Host == "" {
		req.Host = "localhost:8090"
	}
	return req
}

func TestRequireRole(t *testing.T) {
	t.Parallel()
	app := fiber.New()
	app.Get("/admin", func(c *fiber.Ctx) error {
		c.Locals("user", auth.UserDTO{ID: 1, Username: "contestant", Role: auth.RoleContestant})
		return c.Next()
	}, requireRole(auth.RoleAdmin), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusNoContent)
	})

	response, err := app.Test(mustRequest(t, http.MethodGet, "/admin", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("contestant reached admin route: %d", response.StatusCode)
	}
}

func TestSocketControllersRegister(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	sessions := auth.NewService(database)
	if _, err := sessions.EnsureFixedContestants(ctx); err != nil {
		t.Fatal(err)
	}
	fixed, err := sessions.ListFixedContestants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	games, err := game.NewService(ctx, database, fixed, nil)
	if err != nil {
		t.Fatal(err)
	}
	siteSettings, err := settings.Open(filepath.Join(t.TempDir(), "settings.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	sounds, err := sound.NewService(ctx, database, siteSettings, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := registerSocketControllers(opsocket.NewSocket(), games, sounds); err != nil {
		t.Fatal(err)
	}
}

func TestListenRefusesInsecureNonLoopback(t *testing.T) {
	server := &Server{address: "0.0.0.0:8090"}
	if err := server.Listen(); err == nil {
		t.Fatal("expected insecure non-loopback listener to be refused")
	}
}
