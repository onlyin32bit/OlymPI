package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/config"
	"github.com/onlyin32bit/4t/internal/cue"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
	"github.com/onlyin32bit/4t/internal/settings"
	applog "github.com/onlyin32bit/4t/internal/shared/logger"
	"github.com/onlyin32bit/4t/internal/sound"
	store "github.com/onlyin32bit/4t/internal/store/sqlite"
	"github.com/onlyin32bit/4t/internal/web"
	"golang.org/x/term"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type App struct {
	store  *store.Store
	server *web.Server
	games  *game.Service
}

func New(ctx context.Context, cfg config.Config, input io.Reader, output io.Writer) (*App, error) {
	applog.Info(ctx, "opening application database")
	database, err := store.Open(ctx, filepath.Join(cfg.DataDir, "4t.db"))
	if err != nil {
		applog.Error(ctx, "application database open failed", "error", err)
		return nil, err
	}
	applog.Info(ctx, "application database opened")

	systemSettings, err := config.OpenSystemSettings(ctx, database)
	if err != nil {
		database.Close()
		return nil, err
	}
	runtimeConfig := cfg
	persistedSystem := systemSettings.Get()
	if !runtimeConfig.Server.BindSet {
		runtimeConfig.Server.Bind = persistedSystem.Bind
	}
	if !runtimeConfig.Server.PortSet {
		runtimeConfig.Server.Port = persistedSystem.Port
	}
	// TLS always requires Secure cookies. Only the explicit insecure opt-in
	// may override a persisted Secure-cookie policy for a plain HTTP launch.
	runtimeConfig.Server.SecureCookies = runtimeConfig.Server.TLSCertFile != "" ||
		((runtimeConfig.Server.SecureCookies || persistedSystem.SecureCookies) && !runtimeConfig.Server.AllowInsecureLAN)
	if err := runtimeConfig.ValidateServer(); err != nil {
		database.Close()
		return nil, fmt.Errorf("server configuration: %w", err)
	}

	sessions := auth.NewService(database)
	credentials, err := sessions.EnsureFixedContestants(ctx)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("ensure fixed contestant accounts: %w", err)
	}
	if len(credentials) > 0 {
		fmt.Fprintln(output, "Created fixed contestant accounts. Save these one-time passwords and reset them in Settings:")
		for _, credential := range credentials {
			fmt.Fprintf(output, "  %s: %s\n", credential.Username, credential.Password)
		}
	}

	socket := opsocket.NewSocket()
	socket.SetDebug(runtimeConfig.Server.Debug)
	siteSettings, err := settings.OpenWithRepository(ctx, filepath.Join(cfg.DataDir, "settings.yaml"), database)
	if err != nil {
		database.Close()
		return nil, err
	}
	sounds, err := sound.NewService(ctx, database, siteSettings, func(room, event string, data any) {
		socket.EmitEventToRoom(room, event, data)
	}, func() int { return socket.RoomSize(sound.OutputRoom) })
	if err != nil {
		database.Close()
		return nil, err
	}
	fixedUsers, err := sessions.ListFixedContestants(ctx)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("load fixed contestant accounts: %w", err)
	}
	games, err := game.NewService(ctx, database, fixedUsers, func(room, event string, data any) {
		socket.EmitEventToRoom(room, event, data)
	})
	if err != nil {
		database.Close()
		return nil, err
	}
	// Server-side sound cue director (legacy audience sounds from state changes).
	director := cue.New(sounds, func() cue.Settings {
		values := siteSettings.Get().Sound
		return cue.Settings{Enabled: values.Enabled, BellSoundID: values.BellSoundID}
	})
	games.SetObserver(director.Observe)
	server := web.New(runtimeConfig, sessions, siteSettings, games, sounds, socket, systemSettings)

	applog.Info(ctx, "application services initialized")
	return &App{store: database, server: server, games: games}, nil
}
func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	applog.Info(ctx, "starting HTTP server")
	go func() { errCh <- a.server.Listen() }()
	go a.openBrowserWhenReady(ctx)

	select {
	case <-ctx.Done():
		applog.Info(ctx, "shutting down HTTP server")
		err := a.server.Shutdown()
		if a.games != nil {
			// Pause the live timer so a restart resumes with the time that was left.
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if pauseErr := a.games.Shutdown(shutdownCtx); pauseErr != nil {
				applog.Error(ctx, "could not pause the match timer", "error", pauseErr)
			}
			cancel()
		}
		return err
	case err := <-errCh:
		if err != nil {
			applog.Error(ctx, "HTTP server stopped", "error", err)
		}
		return err
	}
}

func (a *App) openBrowserWhenReady(ctx context.Context) {
	if a.server == nil || a.server.NoBrowser() {
		return
	}
	address := a.server.BrowserAddress()
	for attempt := 0; attempt < 50; attempt++ {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			if err := openBrowser(a.server.BrowserURL()); err != nil {
				applog.Warn(ctx, "could not open browser automatically", "error", err)
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func openBrowser(url string) error {
	var command string
	var args []string
	switch {
	case os.Getenv("OS") == "Windows_NT":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case strings.Contains(strings.ToLower(os.Getenv("OSTYPE")), "darwin"):
		command, args = "open", []string{url}
	default:
		command, args = "xdg-open", []string{url}
	}
	return exec.Command(command, args...).Start()
}

func (a *App) Close() error {
	if a.store == nil {
		return nil
	}
	return a.store.Close()
}

func ensureInitialAdmin(ctx context.Context, service *auth.Service, input io.Reader, output io.Writer) error {
	hasAdmin, err := service.HasAdmin(ctx)
	if err != nil {
		return fmt.Errorf("check initial administrator: %w", err)
	}
	if hasAdmin {
		applog.Debug(ctx, "initial administrator already exists")
		return nil
	}

	file, ok := input.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return errors.New("no administrator exists; run 4t once from an interactive terminal to create one")
	}

	fmt.Fprintln(output, "No administrator exists. Create the initial administrator.")
	reader := bufio.NewReader(file)
	var username string
	for username == "" {
		fmt.Fprint(output, "Username: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read administrator username: %w", err)
		}
		username = strings.TrimSpace(line)
	}

	for {
		fmt.Fprint(output, "Password (minimum 8 characters): ")
		password, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(output)
		if err != nil {
			return fmt.Errorf("read administrator password: %w", err)
		}
		if len(password) < 8 {
			fmt.Fprintln(output, "Password is too short.")
			continue
		}

		fmt.Fprint(output, "Confirm password: ")
		confirmation, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(output)
		if err != nil {
			return fmt.Errorf("confirm administrator password: %w", err)
		}
		if string(password) != string(confirmation) {
			fmt.Fprintln(output, "Passwords do not match.")
			continue
		}

		if _, err := service.CreateAdmin(ctx, username, string(password)); err != nil {
			return fmt.Errorf("create initial administrator: %w", err)
		}
		fmt.Fprintln(output, "Administrator created.")
		return nil
	}
}
