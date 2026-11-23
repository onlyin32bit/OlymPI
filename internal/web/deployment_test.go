package web

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/config"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
	"github.com/onlyin32bit/4t/internal/settings"
	"github.com/onlyin32bit/4t/internal/sound"
	store "github.com/onlyin32bit/4t/internal/store/sqlite"
)

func deploymentServer(t *testing.T) (*Server, *game.Service, string) {
	t.Helper()
	return deploymentServerWithRepository(t, nil)
}

func deploymentServerWithRepository(t *testing.T, wrap func(game.Repository) game.Repository) (*Server, *game.Service, string) {
	t.Helper()
	cfg, err := config.Load([]string{"--bind", "0.0.0.0", "--port", "8443", "--tls-cert", "cert.pem", "--tls-key", "key.pem", "--allowed-host", "192.0.2.10", "--data-dir", filepath.Join(t.TempDir(), "custom-data")})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Server.ServeUI = false
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(cfg.DataDir, "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	sessions := auth.NewService(database)
	if _, err := sessions.CreateAdmin(ctx, "admin", "password123"); err != nil {
		t.Fatal(err)
	}
	siteSettings, err := settings.OpenWithRepository(ctx, filepath.Join(cfg.DataDir, "settings.yaml"), database)
	if err != nil {
		t.Fatal(err)
	}
	var repository game.Repository = database
	if wrap != nil {
		repository = wrap(repository)
	}
	games, err := game.NewService(ctx, repository, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sounds, err := sound.NewService(ctx, database, siteSettings, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	system, err := config.OpenSystemSettings(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg, sessions, siteSettings, games, sounds, opsocket.NewSocket(), system), games, cfg.DataDir
}

func deploymentRequest(t *testing.T, server *Server, method, path string, body io.Reader, contentType string, cookie *http.Cookie, want int) []byte {
	t.Helper()
	request := mustRequest(t, method, "https://192.0.2.10:8443"+path, body)
	request.Header.Set("Origin", "https://192.0.2.10:8443")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := server.app.Test(request, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != want {
		t.Fatalf("%s %s returned %d, want %d: %s", method, path, response.StatusCode, want, contents)
	}
	return contents
}

func deploymentLogin(t *testing.T, server *Server) *http.Cookie {
	t.Helper()
	form := url.Values{"username": {"admin"}, "password": {"password123"}}
	request := mustRequest(t, http.MethodPost, "https://192.0.2.10:8443/api/login", strings.NewReader(form.Encode()))
	request.Header.Set("Origin", "https://192.0.2.10:8443")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := server.app.Test(request, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || len(response.Cookies()) != 1 {
		t.Fatalf("LAN login failed: status %d, cookies %v", response.StatusCode, response.Cookies())
	}
	cookie := response.Cookies()[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("HTTPS session protections missing: %+v", cookie)
	}
	return cookie
}

func deploymentUpload(t *testing.T, server *Server, cookie *http.Cookie, path, filename string, contents []byte, metadata string) []byte {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(contents); err != nil {
		t.Fatal(err)
	}
	if metadata != "" {
		if err := form.WriteField("metadata", metadata); err != nil {
			t.Fatal(err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	return deploymentRequest(t, server, http.MethodPost, path, &body, form.FormDataContentType(), cookie, http.StatusCreated)
}

func TestLANAssetsAndArchivesUseConfiguredDataDirectory(t *testing.T) {
	server, games, dataDir := deploymentServer(t)
	cookie := deploymentLogin(t, server)
	assetData := []byte("\x89PNG\r\n\x1a\ncustom-directory-image")
	var uploaded struct {
		Asset game.Asset `json:"asset"`
	}
	if err := json.Unmarshal(deploymentUpload(t, server, cookie, "/api/assets", "image.png", assetData, ""), &uploaded); err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(filepath.Join(dataDir, "assets", uploaded.Asset.SHA256))
	if err != nil || !bytes.Equal(stored, assetData) {
		t.Fatalf("asset not stored in configured directory: %q, %v", stored, err)
	}
	if got := deploymentRequest(t, server, http.MethodGet, "/api/assets/"+uploaded.Asset.ID, nil, "", nil, http.StatusOK); !bytes.Equal(got, assetData) {
		t.Fatal("public asset route did not read the configured directory")
	}
	questionData := json.RawMessage(`{"kd_chung":[],"kd_rieng":[[],[],[],[]],"vcnv":{"obstacle":"A","obstacle_asset_id":"` + uploaded.Asset.ID + `","rows":[{},{},{},{}],"center":{}},"tt":[],"vd":[]}`)
	set, err := games.SaveQuestionSet(context.Background(), "", game.SaveQuestionSetInput{Name: "custom directory", Data: questionData}, game.Actor{})
	if err != nil {
		t.Fatal(err)
	}
	archive := deploymentRequest(t, server, http.MethodGet, "/api/question-sets/"+set.ID+"/export", nil, "", cookie, http.StatusOK)
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range reader.File {
		if entry.Name == "assets/"+uploaded.Asset.ID {
			contents, err := readZipEntry(entry, maxAssetSize)
			if err != nil || !bytes.Equal(contents, assetData) {
				t.Fatalf("exported asset differs: %q, %v", contents, err)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("export omitted the configured-directory asset")
	}
	otherServer, _, otherDataDir := deploymentServer(t)
	otherCookie := deploymentLogin(t, otherServer)
	deploymentUpload(t, otherServer, otherCookie, "/api/question-sets/import", "questions.zip", archive, "")
	imported, err := os.ReadFile(filepath.Join(otherDataDir, "assets", uploaded.Asset.SHA256))
	if err != nil || !bytes.Equal(imported, assetData) {
		t.Fatalf("import did not use destination data directory: %q, %v", imported, err)
	}
	audio := []byte("ID3\x03\x00\x00\x00\x00\x00\x00custom-directory-audio")
	deploymentUpload(t, server, cookie, "/api/sounds", "sound.mp3", audio, `{"name":"custom sound","category":"test","bus":"effects","volume":1,"enabled":true}`)
	if contents, err := os.ReadFile(filepath.Join(dataDir, "assets", digestForTest(audio))); err != nil || !bytes.Equal(contents, audio) {
		t.Fatalf("sound upload ignored configured data directory: %q, %v", contents, err)
	}
	for _, name := range []string{"4t.db", "settings.yaml"} {
		if _, err := os.Stat(filepath.Join(dataDir, name)); err != nil {
			t.Fatalf("%s not in configured data directory: %v", name, err)
		}
	}
}

func TestLANRejectsForeignOriginOnLoginSetupAndAuthenticatedMutation(t *testing.T) {
	server, _, _ := deploymentServer(t)
	cookie := deploymentLogin(t, server)
	for _, test := range []struct{ method, path string }{
		{http.MethodPost, "/api/login"},
		{http.MethodPost, "/api/setup"},
		{http.MethodPut, "/api/settings"},
	} {
		request := mustRequest(t, test.method, "https://192.0.2.10:8443"+test.path, strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "https://evil.example")
		request.AddCookie(cookie)
		response, err := server.app.Test(request, -1)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("foreign-origin %s status = %d, want 403", test.path, response.StatusCode)
		}
	}
}

func TestBrowserURLUsesHashRoutesAndDialableHosts(t *testing.T) {
	fixture, _, _ := deploymentServer(t)
	for _, test := range []struct {
		address, cert, want string
		setup               bool
	}{
		{"0.0.0.0:8090", "", "http://localhost:8090/#/setup", true},
		{"[::]:8443", "cert.pem", "https://localhost:8443/#/setup", true},
		{"[::1]:8443", "cert.pem", "https://[::1]:8443/#/", false},
		{"[2001:db8::10]:8443", "cert.pem", "https://[2001:db8::10]:8443/#/", false},
		{"192.0.2.10:8443", "cert.pem", "https://192.0.2.10:8443/#/", false},
	} {
		server := &Server{address: test.address, tlsCertFile: test.cert}
		if test.setup {
			server.system = fixture.system
		}
		if got := server.BrowserURL(); got != test.want {
			t.Errorf("BrowserURL(%s) = %q, want %q", test.address, got, test.want)
		}
		parsed, err := url.Parse(server.BrowserURL())
		if err != nil || parsed.Host != server.BrowserAddress() {
			t.Errorf("launcher readiness address differs from browser URL: %q, %v", server.BrowserAddress(), err)
		}
	}
}

func TestListenRefusesInvalidTLSAndCookieConfigurations(t *testing.T) {
	for _, server := range []*Server{
		{address: "0.0.0.0:8443", tlsCertFile: "cert.pem"},
		{address: "0.0.0.0:8443", tlsKeyFile: "key.pem"},
		{address: "0.0.0.0:8443", tlsCertFile: "cert.pem", tlsKeyFile: "key.pem", allowInsecureLAN: true},
		{address: "127.0.0.1:8090", secureCookies: true},
	} {
		if err := server.Listen(); err == nil {
			t.Fatalf("unsafe listener configuration accepted: %+v", server)
		}
	}
}

func TestMeClearsInvalidSessionCookieWithoutWeakeningCookieOptions(t *testing.T) {
	server, _, _ := deploymentServer(t)
	request := mustRequest(t, http.MethodGet, "https://192.0.2.10:8443/api/me", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "expired-or-invalid-session"})
	response, err := server.app.Test(request, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid session status = %d, want 401", response.StatusCode)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || cookies[0].Value != "" ||
		cookies[0].Path != "/" || cookies[0].MaxAge > 0 || cookies[0].Expires.IsZero() ||
		!cookies[0].Expires.Before(time.Now()) || !cookies[0].Secure ||
		!cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("invalid session was not cleared with matching secure cookie options: %v", cookies)
	}
}

func TestLANSetupIsReachableAndUpdatesLauncherRoute(t *testing.T) {
	server, _, _ := deploymentServer(t)
	if got := server.BrowserURL(); got != "https://localhost:8443/#/setup" {
		t.Fatalf("first-run launcher URL = %q", got)
	}
	deploymentRequest(t, server, http.MethodGet, "/api/setup/status", nil, "", nil, http.StatusOK)
	deploymentRequest(t, server, http.MethodPost, "/api/setup",
		strings.NewReader(`{"bind":"0.0.0.0","port":8443,"secure_cookies":true}`),
		"application/json", nil, http.StatusOK)
	if !server.system.Configured() || server.system.Get().Bind != "0.0.0.0" {
		t.Fatalf("LAN setup settings not persisted: %+v", server.system.Get())
	}
	if got := server.BrowserURL(); got != "https://localhost:8443/#/" {
		t.Fatalf("configured launcher URL = %q", got)
	}
}
