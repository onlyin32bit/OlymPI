package config

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	applog "github.com/onlyin32bit/4t/internal/shared/logger"
)

type Config struct {
	Server struct {
		Port             int
		PortSet          bool
		Bind             string
		BindSet          bool
		ServeUI          bool
		Debug            bool
		LogLevel         string
		TLSCertFile      string
		TLSKeyFile       string
		SecureCookies    bool
		AllowInsecureLAN bool
		AllowedHosts     []string
	}
	DataDir   string
	NoBrowser bool
}

type SystemSettings struct {
	Bind          string `json:"bind"`
	Port          int    `json:"port"`
	SecureCookies bool   `json:"secure_cookies"`
	SetupComplete bool   `json:"setup_complete"`
}

func DefaultSystemSettings() SystemSettings {
	return SystemSettings{Bind: "127.0.0.1", Port: 8090}
}

type SystemRepository interface {
	LoadSystemSettings(context.Context) (SystemSettings, error)
	SaveSystemSettings(context.Context, SystemSettings) error
}

var ErrNotFound = errors.New("system settings not found")

type SystemService struct {
	mu     sync.RWMutex
	repo   SystemRepository
	values SystemSettings
}

func OpenSystemSettings(ctx context.Context, repo SystemRepository) (*SystemService, error) {
	if repo == nil {
		return nil, errors.New("system settings repository is required")
	}
	values, err := repo.LoadSystemSettings(ctx)
	values.Bind = strings.TrimSpace(values.Bind)
	if errors.Is(err, ErrNotFound) {
		values = DefaultSystemSettings()
	} else if err != nil {
		return nil, fmt.Errorf("load system settings: %w", err)
	} else if err := validateSystemSettings(values); err != nil {
		return nil, fmt.Errorf("validate system settings: %w", err)
	}
	return &SystemService{repo: repo, values: values}, nil
}

func (s *SystemService) Get() SystemSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.values
}

func (s *SystemService) Update(ctx context.Context, values SystemSettings) (SystemSettings, error) {
	values.Bind = strings.TrimSpace(values.Bind)
	if err := validateSystemSettings(values); err != nil {
		return SystemSettings{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.repo.SaveSystemSettings(ctx, values); err != nil {
		return SystemSettings{}, err
	}
	s.values = values
	return values, nil
}

func (s *SystemService) Configured() bool {
	return s.Get().SetupComplete
}

func validateSystemSettings(values SystemSettings) error {
	values.Bind = strings.TrimSpace(values.Bind)
	if values.Bind != "localhost" && net.ParseIP(values.Bind) == nil {
		return errors.New("bind must be localhost or an IP address (without brackets or a port)")
	}
	if values.Port < 1 || values.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	return nil
}

// ValidateServer checks the effective configuration after persisted settings and
// command-line overrides have been combined.
func (cfg Config) ValidateServer() error {
	if err := validateSystemSettings(SystemSettings{Bind: cfg.Server.Bind, Port: cfg.Server.Port}); err != nil {
		return err
	}
	for _, host := range cfg.Server.AllowedHosts {
		if !validAllowedHost(host) {
			return fmt.Errorf("invalid --allowed-host %q: use an exact IP address or hostname without a scheme, wildcard, or port", host)
		}
	}
	hasCert := cfg.Server.TLSCertFile != ""
	hasKey := cfg.Server.TLSKeyFile != ""
	if hasCert != hasKey {
		return errors.New("--tls-cert and --tls-key must be supplied together")
	}
	if hasCert {
		if cfg.Server.AllowInsecureLAN {
			return errors.New("--allow-insecure-lan cannot be combined with TLS")
		}
		return nil
	}
	if cfg.Server.SecureCookies {
		return errors.New("secure cookies require TLS; configure --tls-cert and --tls-key")
	}
	ip := net.ParseIP(cfg.Server.Bind)
	if cfg.Server.Bind != "localhost" && (ip == nil || !ip.IsLoopback()) && !cfg.Server.AllowInsecureLAN {
		return errors.New("non-loopback HTTP requires --allow-insecure-lan; prefer --tls-cert and --tls-key")
	}
	return nil
}

func validAllowedHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if net.ParseIP(host) != nil {
		return true
	}
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if ch != '-' && (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') {
				return false
			}
		}
	}
	return true
}

func Load(args []string) (Config, error) {
	var cfg Config
	cfg.Server.Port = 8090
	cfg.Server.Bind = "127.0.0.1"
	cfg.Server.ServeUI = true
	cfg.DataDir = "4t_data"
	flags := flag.NewFlagSet("4t", flag.ContinueOnError)
	bind := flags.String("bind", cfg.Server.Bind, "address to bind the server to")
	port := flags.Int("port", cfg.Server.Port, "port to bind the server to")
	noServeUI := flags.Bool("no-serve-ui", false, "do not serve the embedded UI")
	debug := flags.Bool("debug", false, "enable debug logging")
	dataDir := flags.String("data-dir", cfg.DataDir, "directory for the application database and assets")
	noBrowser := flags.Bool("no-browser", false, "do not open the setup or application page automatically")
	tlsCert := flags.String("tls-cert", "", "PEM TLS certificate file (requires --tls-key)")
	tlsKey := flags.String("tls-key", "", "PEM TLS private key file (requires --tls-cert)")
	allowInsecureLAN := flags.Bool("allow-insecure-lan", false, "allow unencrypted LAN HTTP and non-Secure cookies (credentials and game traffic are exposed)")
	flags.Func("allowed-host", "additional trusted request hostname or IP, without scheme/port (repeat for multiple names)", func(value string) error {
		value = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
		if !validAllowedHost(value) {
			return fmt.Errorf("invalid allowed host %q", value)
		}
		cfg.Server.AllowedHosts = append(cfg.Server.AllowedHosts, value)
		return nil
	})
	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}
	cfg.DataDir = strings.TrimSpace(*dataDir)
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "bind":
			cfg.Server.BindSet = true
		case "port":
			cfg.Server.PortSet = true
		}
	})
	cfg.Server.Bind = strings.TrimSpace(*bind)
	cfg.Server.Port = *port
	cfg.Server.TLSCertFile = strings.TrimSpace(*tlsCert)
	cfg.Server.TLSKeyFile = strings.TrimSpace(*tlsKey)
	cfg.Server.SecureCookies = cfg.Server.TLSCertFile != ""
	cfg.Server.AllowInsecureLAN = *allowInsecureLAN
	if err := cfg.ValidateServer(); err != nil {
		return Config{}, err
	}
	if cfg.DataDir == "" {
		return Config{}, errors.New("data directory is required")
	}
	if envLevel := strings.TrimSpace(os.Getenv("LOG_LEVEL")); envLevel != "" {
		cfg.Server.LogLevel = envLevel
	}
	normalizedLevel, err := applog.NormalizeLevel(cfg.Server.LogLevel)
	if err != nil {
		return Config{}, err
	}
	cfg.Server.LogLevel = normalizedLevel
	if *debug {
		cfg.Server.Debug = true
		cfg.Server.LogLevel = "debug"
	}
	cfg.Server.ServeUI = !*noServeUI
	cfg.NoBrowser = *noBrowser
	return cfg, nil
}
