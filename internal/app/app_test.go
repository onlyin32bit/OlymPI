package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/config"
	store "github.com/onlyin32bit/4t/internal/store/sqlite"
)

func TestInitialAdminRequiresInteractiveTerminal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	err = ensureInitialAdmin(ctx, auth.NewService(database), strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("expected interactive terminal error, got %v", err)
	}
}

func TestInitialAdminSkipsPromptWhenAdminExists(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := auth.NewService(database)
	if _, err := service.CreateAdmin(ctx, "admin", "password123"); err != nil {
		t.Fatal(err)
	}

	if err := ensureInitialAdmin(ctx, service, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatalf("existing administrator should skip prompt: %v", err)
	}
}

func TestNewServerFlagsOverridePersistedSettings(t *testing.T) {
	ctx := context.Background()
	persisted := config.SystemSettings{
		Bind:          "localhost",
		Port:          8091,
		SecureCookies: true,
	}

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "omitted", want: "localhost:8091"},
		{name: "explicit", args: []string{"--bind", "127.0.0.1", "--port", "8092"}, want: "127.0.0.1:8092"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			database, err := store.Open(ctx, filepath.Join(dataDir, "4t.db"))
			if err != nil {
				t.Fatal(err)
			}
			if err := database.SaveSystemSettings(ctx, persisted); err != nil {
				database.Close()
				t.Fatal(err)
			}
			if err := database.Close(); err != nil {
				t.Fatal(err)
			}

			args := append([]string{"--tls-cert", "cert.pem", "--tls-key", "key.pem"}, test.args...)
			cfg, err := config.Load(args)
			if err != nil {
				t.Fatal(err)
			}
			cfg.DataDir = dataDir
			cfg.Server.ServeUI = false

			application, err := New(ctx, cfg, strings.NewReader(""), &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			defer application.Close()

			if got := application.server.Address(); got != test.want {
				t.Fatalf("server address = %q, want %q", got, test.want)
			}
			if got, err := application.store.LoadSystemSettings(ctx); err != nil {
				t.Fatal(err)
			} else if got != persisted {
				t.Fatalf("persisted system settings changed: got %+v, want %+v", got, persisted)
			}
		})
	}
}

func TestNewValidatesPersistedLANTransportAndHonorsExplicitOptions(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name      string
		args      []string
		bind      string
		secure    bool
		wantError bool
		wantURL   string
	}{
		{name: "persisted LAN needs opt-in", bind: "0.0.0.0", wantError: true},
		{name: "LAN TLS", bind: "0.0.0.0", args: []string{"--tls-cert", "cert.pem", "--tls-key", "key.pem"}, wantURL: "https://localhost:8091/#/setup"},
		{name: "explicit insecure LAN", bind: "0.0.0.0", args: []string{"--allow-insecure-lan"}, wantURL: "http://localhost:8091/#/setup"},
		{name: "secure policy needs TLS", bind: "127.0.0.1", secure: true, wantError: true},
		{name: "secure policy TLS", bind: "127.0.0.1", secure: true, args: []string{"--tls-cert", "cert.pem", "--tls-key", "key.pem"}, wantURL: "https://127.0.0.1:8091/#/setup"},
		{name: "explicit insecure overrides persisted cookie policy", bind: "0.0.0.0", secure: true, args: []string{"--allow-insecure-lan"}, wantURL: "http://localhost:8091/#/setup"},
		{name: "explicit loopback overrides persisted LAN", bind: "0.0.0.0", args: []string{"--bind", "::1", "--port", "8092"}, wantURL: "http://[::1]:8092/#/setup"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dataDir := filepath.Join(t.TempDir(), "data")
			database, err := store.Open(ctx, filepath.Join(dataDir, "4t.db"))
			if err != nil {
				t.Fatal(err)
			}
			persisted := config.SystemSettings{Bind: test.bind, Port: 8091, SecureCookies: test.secure}
			if err := database.SaveSystemSettings(ctx, persisted); err != nil {
				database.Close()
				t.Fatal(err)
			}
			if err := database.Close(); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"--data-dir", dataDir, "--no-serve-ui", "--no-browser"}, test.args...)
			cfg, err := config.Load(args)
			if err != nil {
				t.Fatal(err)
			}
			application, err := New(ctx, cfg, strings.NewReader(""), &bytes.Buffer{})
			if test.wantError {
				if err == nil {
					application.Close()
					t.Fatal("unsafe persisted transport accepted without explicit opt-in")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer application.Close()
			if got := application.server.BrowserURL(); got != test.wantURL {
				t.Fatalf("launcher URL = %q, want %q", got, test.wantURL)
			}
			if !application.server.NoBrowser() {
				t.Fatal("--no-browser was not honored")
			}
			for _, name := range []string{"4t.db", "settings.yaml"} {
				if _, err := os.Stat(filepath.Join(dataDir, name)); err != nil {
					t.Fatalf("%s not written under --data-dir: %v", name, err)
				}
			}
		})
	}
}
