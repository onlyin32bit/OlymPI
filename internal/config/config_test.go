package config

import (
	"context"
	"testing"
)

type memorySystemRepository struct {
	values *SystemSettings
}

func (m *memorySystemRepository) LoadSystemSettings(context.Context) (SystemSettings, error) {
	if m.values == nil {
		return SystemSettings{}, ErrNotFound
	}
	return *m.values, nil
}
func (m *memorySystemRepository) SaveSystemSettings(_ context.Context, values SystemSettings) error {
	m.values = &values
	return nil
}

func TestLoadDefaultsToLoopbackBind(t *testing.T) {
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Bind != "127.0.0.1" || cfg.Server.Port != 8090 {
		t.Fatalf("unexpected defaults: %+v", cfg.Server)
	}
}

func TestLoadAcceptsExplicitServerFlags(t *testing.T) {
	cfg, err := Load([]string{"--bind", " localhost ", "--port", "8091"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Bind != "localhost" || !cfg.Server.BindSet {
		t.Fatalf("unexpected bind configuration: %+v", cfg.Server)
	}
	if cfg.Server.Port != 8091 || !cfg.Server.PortSet {
		t.Fatalf("unexpected port configuration: %+v", cfg.Server)
	}
}

func TestLoadRejectsInvalidServerFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--bind", "0.0.0.0"},
		{"--bind", "example.com"},
		{"--bind", "[::1]"},
		{"--bind", "127.0.0.1:8090"},
		{"--tls-cert", "cert.pem"},
		{"--tls-key", "key.pem"},
		{"--tls-cert", "cert.pem", "--tls-key", "key.pem", "--allow-insecure-lan"},
		{"--port", "0"},
		{"--port", "65536"},
		{"--allowed-host", "*"},
		{"--allowed-host", "*.example.com"},
		{"--allowed-host", "https://game.example"},
		{"--allowed-host", "game.example:8443"},
		{"--allowed-host", "user@game.example"},
		{"--allowed-host", ""},
	} {
		if _, err := Load(args); err == nil {
			t.Errorf("Load(%q) succeeded with invalid server configuration", args)
		}
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "verbose")
	if _, err := Load(nil); err == nil {
		t.Fatal("expected invalid log level to fail")
	}
}

func TestSystemSettingsPersistAndValidate(t *testing.T) {
	repo := &memorySystemRepository{}
	service, err := OpenSystemSettings(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	values := DefaultSystemSettings()
	values.Port = 8091
	values.SetupComplete = true
	if _, err := service.Update(context.Background(), values); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSystemSettings(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Get(); got != values {
		t.Fatalf("persisted settings mismatch: got %+v want %+v", got, values)
	}
	values.Bind = "0.0.0.0"
	if _, err := service.Update(context.Background(), values); err != nil {
		t.Fatalf("persisting a LAN bind should be allowed (transport is validated on launch): %v", err)
	}
	values.Bind = "not an IP"
	if _, err := service.Update(context.Background(), values); err == nil {
		t.Fatal("expected invalid bind to fail")
	}
}

func TestLoadLANAndTLSConfiguration(t *testing.T) {
	for _, bind := range []string{"0.0.0.0", "192.168.1.10", "::", "2001:db8::10"} {
		t.Run(bind, func(t *testing.T) {
			cfg, err := Load([]string{"--bind", bind, "--port", "8443", "--tls-cert", " cert.pem ", "--tls-key", " key.pem ", "--data-dir", "/custom/data", "--no-browser"})
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Server.Bind != bind || cfg.Server.Port != 8443 ||
				cfg.Server.TLSCertFile != "cert.pem" || cfg.Server.TLSKeyFile != "key.pem" ||
				!cfg.Server.SecureCookies || cfg.DataDir != "/custom/data" || !cfg.NoBrowser {
				t.Fatalf("configuration flags not honored: %+v", cfg)
			}
			insecure, err := Load([]string{"--bind", bind, "--allow-insecure-lan"})
			if err != nil {
				t.Fatal(err)
			}
			if !insecure.Server.AllowInsecureLAN || insecure.Server.SecureCookies {
				t.Fatalf("explicit insecure mode not honored: %+v", insecure.Server)
			}
		})
	}
	if _, err := Load([]string{"--bind", "::1"}); err != nil {
		t.Fatalf("IPv6 loopback should not require insecure opt-in: %v", err)
	}
}

func TestValidateEffectiveServerRefusesPersistedInsecureLAN(t *testing.T) {
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Server.Bind = "192.168.1.10"
	if err := cfg.ValidateServer(); err == nil {
		t.Fatal("persisted LAN settings bypassed explicit HTTP opt-in")
	}
	cfg.Server.Bind = "127.0.0.1"
	cfg.Server.SecureCookies = true
	if err := cfg.ValidateServer(); err == nil {
		t.Fatal("Secure cookies without HTTPS should fail rather than make login unusable")
	}
}

func TestLoadExplicitAllowedHosts(t *testing.T) {
	cfg, err := Load([]string{"--allowed-host", " Game.Example. ", "--allowed-host", "192.0.2.10", "--allowed-host", "2001:db8::10"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"game.example", "192.0.2.10", "2001:db8::10"}
	if len(cfg.Server.AllowedHosts) != len(want) {
		t.Fatalf("trusted hosts = %v, want %v", cfg.Server.AllowedHosts, want)
	}
	for i, host := range want {
		if cfg.Server.AllowedHosts[i] != host {
			t.Fatalf("trusted hosts = %v, want %v", cfg.Server.AllowedHosts, want)
		}
	}
}
