package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/config"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
	"github.com/onlyin32bit/4t/internal/settings"
	applog "github.com/onlyin32bit/4t/internal/shared/logger"
	"github.com/onlyin32bit/4t/internal/sound"
	"github.com/onlyin32bit/4t/ui"
)

const sessionCookieName = "4t-auth-session"

type Server struct {
	app              *fiber.App
	address          string
	tlsCertFile      string
	tlsKeyFile       string
	noBrowser        bool
	system           *config.SystemService
	allowInsecureLAN bool
	secureCookies    bool
	assetWrites      *sync.Mutex
}

func New(cfg config.Config, sessions *auth.Service, siteSettings *settings.Service, games *game.Service, sounds *sound.Service, socket *opsocket.Socket, systemSettings ...*config.SystemService) *Server {
	var system *config.SystemService
	if len(systemSettings) > 0 {
		system = systemSettings[0]
	}
	assetDir := filepath.Join(cfg.DataDir, "assets")
	assetWrites := &sync.Mutex{}
	secureCookies := cfg.Server.TLSCertFile != "" ||
		(cfg.Server.SecureCookies && !cfg.Server.AllowInsecureLAN)
	app := fiber.New(fiber.Config{
		AppName:           "4t",
		ServerHeader:      "4t",
		BodyLimit:         16 * 1024 * 1024,
		EnablePrintRoutes: false,
	})
	app.Use(recover.New())
	app.Use(requestLogMiddleware)
	app.Use(requireTrustedHost(cfg))
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "http://localhost:5173,http://127.0.0.1:5173,http://localhost:8090,http://127.0.0.1:8090",
		AllowHeaders:     "Content-Type,Accept",
		AllowCredentials: true,
	}))

	socket.AttachToFiberWithOptions(app, "/ws", opsocket.AttachOptions{
		AllowAnonymous:             true,
		RejectAuthenticationErrors: true,
		OriginAllowed:              mutationOriginAllowed,
		ValidateAuthenticated: func(ctx context.Context, principal opsocket.Principal) bool {
			_, ok := sessions.ValidateSession(ctx, principal.SessionID)
			return ok
		},
		Authenticate: func(c *fiber.Ctx) (opsocket.Principal, error) {
			token := c.Cookies(sessionCookieName)
			if token == "" {
				return opsocket.Principal{}, nil
			}
			user, ok := sessions.ValidateSession(c.UserContext(), token)
			if !ok {
				return opsocket.Principal{}, errors.New("invalid session")
			}
			principal := opsocket.Principal{Subject: fmt.Sprint(user.ID), Roles: []string{string(user.Role)}, SessionID: token, Claims: map[string]string{"session_id": sessions.SessionID(token)}}
			if user.FixedSlot > 0 {
				principal.Claims["slot"] = fmt.Sprint(user.FixedSlot)
			}
			return principal, nil
		},
	})
	socket.Use(func(operation string, next opsocket.OperationHandler) opsocket.OperationHandler {
		return func(ctx *opsocket.Ctx, payload json.RawMessage) (any, error) {
			principal := ctx.Principal()
			if principal.Authenticated() {
				if _, ok := sessions.ValidateSession(ctx.Context(), principal.SessionID); !ok {
					socket.DisconnectSession(principal.Claims["session_id"])
					return nil, &opsocket.Error{Code: "UNAUTHORIZED", Message: "session expired"}
				}
			}
			return next(ctx, payload)
		}
	})
	if err := registerSocketControllers(socket, games, sounds, siteSettings); err != nil {
		panic(err)
	}

	api := app.Group("/api", func(c *fiber.Ctx) error {
		if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead && c.Method() != fiber.MethodOptions {
			if origin := c.Get("Origin"); origin != "" && !mutationOriginAllowed(c, origin) {
				return fiber.NewError(fiber.StatusForbidden, "cross-origin mutation rejected")
			}
		}
		return c.Next()
	}, serializeAssetWrites(assetWrites))
	registerSetupRoutes(api, sessions, system)
	registerPublicSettingsRoute(api, siteSettings)
	registerPublicGameRoutes(api, games, assetDir)
	registerPublicSoundRoutes(api, sounds)
	private := registerAuthRoutes(api, sessions, socket, secureCookies)
	registerSettingsWriteRoute(private, siteSettings, games, sounds, socket)
	registerSystemSettingsRoutes(private, system)
	registerGameRoutes(private, games, sessions, socket, assetDir)
	registerSoundRoutes(private, sounds, games, assetDir)

	// Legacy bookmarks: the audience display lives in the hash-routed SPA.
	app.Get("/display", redirectToDisplay)
	app.Get("/display/", redirectToDisplay)

	if cfg.Server.ServeUI {
		applog.Info(context.Background(), "serving embedded UI")
		isUIPath := func(c *fiber.Ctx) bool {
			return !strings.HasPrefix(c.Path(), "/api") && c.Path() != "/ws"
		}
		// Many LAN devices load the app over Wi-Fi: compress text assets and let the browser keep
		// the content-hashed build files.
		app.Use(compress.New(compress.Config{
			Level: compress.LevelBestSpeed,
			Next:  func(c *fiber.Ctx) bool { return !isUIPath(c) || strings.HasPrefix(c.Path(), "/sounds") },
		}))
		app.Use(func(c *fiber.Ctx) error {
			if strings.HasPrefix(c.Path(), "/_app/immutable/") {
				c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
			} else if isUIPath(c) && !strings.HasPrefix(c.Path(), "/sounds") && !strings.HasPrefix(c.Path(), "/fonts") {
				// The HTML shell names the current chunk hashes; a stale copy asks for deleted files.
				c.Set(fiber.HeaderCacheControl, "no-cache")
			}
			return c.Next()
		})
		app.All("/*", filesystem.New(filesystem.Config{
			Next: func(c *fiber.Ctx) bool {
				return strings.HasPrefix(c.Path(), "/api") || c.Path() == "/ws"
			},
			Root:         ui.Dist(),
			NotFoundFile: "index.html",
			Index:        "index.html",
		}))
	}

	bind := strings.TrimSpace(cfg.Server.Bind)
	if bind == "" {
		bind = "127.0.0.1"
	}
	return &Server{
		app:              app,
		address:          net.JoinHostPort(bind, strconv.Itoa(cfg.Server.Port)),
		tlsCertFile:      cfg.Server.TLSCertFile,
		tlsKeyFile:       cfg.Server.TLSKeyFile,
		noBrowser:        cfg.NoBrowser,
		system:           system,
		allowInsecureLAN: cfg.Server.AllowInsecureLAN,
		secureCookies:    secureCookies,
		assetWrites:      assetWrites,
	}
}

func redirectToDisplay(c *fiber.Ctx) error {
	return c.Redirect("/#/display", fiber.StatusFound)
}

func (s *Server) Address() string { return s.address }
func (s *Server) NoBrowser() bool { return s.noBrowser }

// BrowserAddress is a local, dialable address even when listening on all interfaces.
func (s *Server) BrowserAddress() string {
	host, port, err := net.SplitHostPort(s.address)
	if err != nil {
		return s.address
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = "localhost"
	}
	return net.JoinHostPort(host, port)
}

func (s *Server) BrowserURL() string {
	path := "/#/"
	if s.system != nil && !s.system.Configured() {
		path = "/#/setup"
	}
	scheme := "http"
	if s.tlsCertFile != "" {
		scheme = "https"
	}
	return scheme + "://" + s.BrowserAddress() + path
}

func (s *Server) Listen() error {
	host, port, err := net.SplitHostPort(s.address)
	if err != nil {
		return fmt.Errorf("parse listen address: %w", err)
	}
	var cfg config.Config
	cfg.Server.Bind = host
	cfg.Server.Port, err = strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("parse listen port: %w", err)
	}
	cfg.Server.TLSCertFile = s.tlsCertFile
	cfg.Server.TLSKeyFile = s.tlsKeyFile
	cfg.Server.SecureCookies = s.secureCookies
	cfg.Server.AllowInsecureLAN = s.allowInsecureLAN
	if err := cfg.ValidateServer(); err != nil {
		return fmt.Errorf("refusing listener %q: %w", s.address, err)
	}
	if s.tlsCertFile != "" {
		return s.app.ListenTLS(s.address, s.tlsCertFile, s.tlsKeyFile)
	}
	if s.allowInsecureLAN {
		applog.Warn(context.Background(), "INSECURE LAN HTTP explicitly enabled: passwords, session cookies, and game traffic are unencrypted; use only on a trusted isolated network", slog.String("address", s.address))
	} else {
		applog.Warn(context.Background(), "serving HTTP without TLS on loopback", slog.String("address", s.address))
	}
	return s.app.Listen(s.address)
}

func (s *Server) Shutdown() error { return s.app.Shutdown() }

func registerAuthRoutes(router fiber.Router, sessions *auth.Service, socket *opsocket.Socket, secureCookies bool) fiber.Router {
	router.Post("/login", func(c *fiber.Ctx) error {
		token, expiresAt, err := sessions.LoginFrom(c.UserContext(), c.FormValue("username"), c.FormValue("password"), c.IP(), c.Get(fiber.HeaderUserAgent))
		if errors.Is(err, auth.ErrInvalidCredentials) {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to create session")
		}
		if user, ok := sessions.ValidateSession(c.UserContext(), token); ok && user.Role == auth.RoleContestant {
			socket.DisconnectSubject(fmt.Sprint(user.ID))
		}
		setSessionCookie(c, token, expiresAt, secureCookies || c.Protocol() == "https")
		return c.SendString("logged in")
	})

	private := router.Group("", authenticate(sessions, secureCookies))
	private.Post("/logout", func(c *fiber.Ctx) error {
		token := c.Cookies(sessionCookieName)
		if token == "" {
			return fiber.NewError(fiber.StatusBadRequest, "no auth session found")
		}
		sessionID := sessions.SessionID(token)
		if err := sessions.Logout(c.UserContext(), token); err != nil {
			return err
		}
		socket.DisconnectSession(sessionID)
		deleteSessionCookie(c, secureCookies || c.Protocol() == "https")
		return c.SendString("logged out")
	})
	private.Get("/me", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"user": c.Locals("user")})
	})
	return private
}

func registerPublicSettingsRoute(public fiber.Router, service *settings.Service) {
	public.Get("/settings", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"settings": service.Get()})
	})
}

// SettingsUpdatedEvent tells displays to re-fetch GET /api/settings. It has no
// payload, so it reaches the audience display even with sound output off.
const SettingsUpdatedEvent = "Settings.Updated"

var settingsUpdatedRooms = []string{"public", "role:" + string(auth.RoleAdmin), "role:" + string(auth.RoleContestant)}

func registerSettingsWriteRoute(private fiber.Router, service *settings.Service, games *game.Service, sounds *sound.Service, socket *opsocket.Socket) {
	admin := private.Group("", requireRole(auth.RoleAdmin))
	admin.Put("/settings", func(c *fiber.Ctx) error {
		before := service.Get()
		var values settings.Values
		if err := c.BodyParser(&values); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid settings payload")
		}
		if strings.TrimSpace(values.Sound.BellSoundID) == "" {
			values.Sound = before.Sound
		}
		if values.Scoring == (settings.ScoringValues{}) {
			values.Scoring = before.Scoring
		}
		if !sounds.Exists(c.UserContext(), values.Sound.BellSoundID) {
			return fiber.NewError(fiber.StatusBadRequest, "selected bell sound does not exist or is disabled")
		}
		beforeJSON, _ := json.Marshal(before)
		actor := httpActor(c)
		var updated settings.Values
		var err error
		if service.SupportsAtomicAudit() {
			afterJSON, marshalErr := json.Marshal(values)
			if marshalErr != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "could not encode settings audit")
			}
			updated, err = service.UpdateWithAudit(c.UserContext(), values, settings.AuditEntry{
				Action: "settings.update", EntityType: "settings", EntityID: "global",
				Before: beforeJSON, After: afterJSON, ActorID: actor.UserID, ClientID: actor.ClientID,
			})
		} else {
			updated, err = service.Update(values)
			if err == nil {
				afterJSON, _ := json.Marshal(updated)
				err = games.RecordAudit(c.UserContext(), game.AuditEntry{Action: "settings.update", EntityType: "settings", EntityID: "global", Before: beforeJSON, After: afterJSON}, actor)
				if err != nil {
					if _, rollbackErr := service.Update(before); rollbackErr != nil {
						return fiber.NewError(fiber.StatusInternalServerError, "settings audit failed and the change could not be rolled back")
					}
					return fiber.NewError(fiber.StatusInternalServerError, "settings audit failed; the change was rolled back")
				}
			}
		}
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		sounds.SettingsChanged()
		if socket != nil {
			socket.EmitEventToRooms(settingsUpdatedRooms, SettingsUpdatedEvent, nil)
		}
		return c.JSON(fiber.Map{"settings": updated})
	})
}

type setupRequest struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	Bind          string `json:"bind"`
	Port          int    `json:"port"`
	SecureCookies bool   `json:"secure_cookies"`
}

func registerSetupRoutes(public fiber.Router, sessions *auth.Service, system *config.SystemService) {
	public.Get("/setup/status", func(c *fiber.Ctx) error {
		if system == nil {
			return c.JSON(fiber.Map{"configured": true, "needs_admin": false})
		}
		hasAdmin, err := sessions.HasAdmin(c.UserContext())
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not inspect setup state")
		}
		return c.JSON(fiber.Map{"configured": system.Configured() && hasAdmin, "needs_admin": !hasAdmin, "system": system.Get()})
	})

	public.Post("/setup", func(c *fiber.Ctx) error {
		if system == nil || system.Configured() {
			return fiber.NewError(fiber.StatusConflict, "application setup is already complete")
		}
		var request setupRequest
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid setup payload")
		}
		hasAdmin, err := sessions.HasAdmin(c.UserContext())
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not inspect administrator")
		}
		if !hasAdmin {
			if _, err := sessions.CreateAdmin(c.UserContext(), request.Username, request.Password); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, err.Error())
			}
		}
		systemValues := system.Get()
		if request.Bind != "" {
			systemValues.Bind = request.Bind
		}
		if request.Port != 0 {
			systemValues.Port = request.Port
		}
		systemValues.SecureCookies = request.SecureCookies
		systemValues.SetupComplete = true
		if _, err := system.Update(c.UserContext(), systemValues); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"setup_complete": true, "restart_required": true})
	})
}

func registerSystemSettingsRoutes(private fiber.Router, system *config.SystemService) {
	if system == nil {
		return
	}
	admin := private.Group("", requireRole(auth.RoleAdmin))
	admin.Get("/system-settings", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"system": system.Get()})
	})
	admin.Put("/system-settings", func(c *fiber.Ctx) error {
		var values config.SystemSettings
		if err := c.BodyParser(&values); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid system settings payload")
		}
		values.SetupComplete = system.Configured()
		updated, err := system.Update(c.UserContext(), values)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"system": updated, "restart_required": true})
	})
}

func authenticate(sessions *auth.Service, secureCookies bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead && c.Method() != fiber.MethodOptions {
			if origin := c.Get("Origin"); origin != "" && !mutationOriginAllowed(c, origin) {
				return fiber.NewError(fiber.StatusForbidden, "cross-origin mutation rejected")
			}
		}
		token := c.Cookies(sessionCookieName)
		if token == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "missing auth token")
		}
		user, ok := sessions.ValidateSession(c.UserContext(), token)
		if !ok {
			deleteSessionCookie(c, secureCookies || c.Protocol() == "https")
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired session")
		}

		c.Locals("user", user)
		return c.Next()
	}
}
func requestLogMiddleware(c *fiber.Ctx) error {
	started := time.Now()
	err := c.Next()
	status := c.Response().StatusCode()
	if fiberErr, ok := err.(*fiber.Error); ok {
		status = fiberErr.Code
	} else if err != nil && status < fiber.StatusBadRequest {
		status = fiber.StatusInternalServerError
	}
	attrs := []any{
		slog.Int("status", status),
		slog.String("method", c.Method()),
		slog.String("path", c.Path()),
		slog.String("remote", c.IP()),
		slog.Duration("duration", time.Since(started)),
	}
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}
	switch requestLogLevel(status) {
	case slog.LevelError:
		applog.Error(c.UserContext(), "http request", attrs...)
	case slog.LevelWarn:
		applog.Warn(c.UserContext(), "http request", attrs...)
	case slog.LevelInfo:
		applog.Info(c.UserContext(), "http request", attrs...)
	default:
		applog.Debug(c.UserContext(), "http request", attrs...)
	}
	return err
}

func requestLogLevel(status int) slog.Level {
	switch {
	case status >= fiber.StatusInternalServerError:
		return slog.LevelError
	case status >= fiber.StatusBadRequest:
		return slog.LevelWarn
	case status >= fiber.StatusMultipleChoices:
		return slog.LevelInfo
	default:
		return slog.LevelDebug
	}
}

func mutationOriginAllowed(c *fiber.Ctx, rawOrigin string) bool {
	origin, err := url.Parse(rawOrigin)
	if err != nil || origin.Scheme != "http" && origin.Scheme != "https" || origin.Host == "" ||
		origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return false
	}
	if strings.EqualFold(origin.Host, c.Get("Host")) {
		return true
	}

	// Vite serves the UI on 5173 and proxies mutations to the loopback Go
	// server during development. Keep this exception loopback-only.
	requestHost, _, err := net.SplitHostPort(c.Get("Host"))
	if err != nil {
		requestHost = c.Get("Host")
	}
	return origin.Port() == "5173" && isLoopbackHost(origin.Hostname()) && isLoopbackHost(requestHost)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func requireRole(role auth.Role) fiber.Handler {
	return func(c *fiber.Ctx) error {
		user, ok := c.Locals("user").(auth.UserDTO)
		if !ok || user.Role != role {
			return fiber.NewError(fiber.StatusForbidden, "insufficient permissions")
		}
		return c.Next()
	}
}

func setSessionCookie(c *fiber.Ctx, token string, expiresAt time.Time, secure bool) {
	c.Cookie(&fiber.Cookie{
		Name: sessionCookieName, Value: token,
		Expires: expiresAt, Path: "/", HTTPOnly: true,
		Secure: secure, SameSite: fiber.CookieSameSiteLaxMode,
	})
}
