package web

import (
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/onlyin32bit/4t/internal/config"
)

// Trust listener addresses independently of Origin: an attacker can supply a
// matching Host/Origin pair after rebinding its own DNS name to this server.
type requestHostPolicy struct {
	hosts map[string]struct{}
}

func newRequestHostPolicy(cfg config.Config, localIPs []net.IP) requestHostPolicy {
	policy := requestHostPolicy{hosts: make(map[string]struct{})}
	add := func(host string) {
		if ip := net.ParseIP(host); ip != nil {
			host = ip.String()
		}
		policy.hosts[strings.TrimSuffix(strings.ToLower(host), ".")] = struct{}{}
	}
	for _, host := range cfg.Server.AllowedHosts {
		add(strings.TrimSpace(host))
	}
	if bind := net.ParseIP(cfg.Server.Bind); bind != nil {
		if bind.IsUnspecified() {
			for _, ip := range localIPs {
				if !ip.IsUnspecified() {
					add(ip.String())
				}
			}
		} else {
			add(bind.String())
		}
	}
	return policy
}

func (p requestHostPolicy) allows(authority string) bool {
	u, err := url.Parse("//" + authority)
	if err != nil || u.Host == "" || u.Host != authority || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" || strings.HasSuffix(authority, ":") {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(authority, "[") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	if isLoopbackHost(host) {
		return true
	}
	_, allowed := p.hosts[host]
	return allowed
}

func requireTrustedHost(cfg config.Config) fiber.Handler {
	var localIPs []net.IP
	if addresses, err := net.InterfaceAddrs(); err == nil {
		for _, address := range addresses {
			if network, ok := address.(*net.IPNet); ok {
				localIPs = append(localIPs, network.IP)
			}
		}
	}
	policy := newRequestHostPolicy(cfg, localIPs)
	return func(c *fiber.Ctx) error {
		// Never use forwarded Host headers to decide which authority is trusted.
		if !policy.allows(c.Get(fiber.HeaderHost)) {
			return fiber.NewError(fiber.StatusForbidden, "untrusted request host")
		}
		return c.Next()
	}
}

func isAssetWrite(method, requestPath string) bool {
	if method == fiber.MethodGet || method == fiber.MethodHead || method == fiber.MethodOptions {
		return false
	}
	// Fiber routes are case-insensitive and accept a trailing slash by default.
	requestPath = strings.ToLower(path.Clean(requestPath))
	for _, prefix := range []string{"/api/assets", "/api/question-sets", "/api/sounds"} {
		if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
			return true
		}
	}
	return false
}

func serializeAssetWrites(lock *sync.Mutex) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if isAssetWrite(c.Method(), c.Path()) {
			// Keep publication, reference adoption, commit and rollback in one
			// critical section. Read-only media/export requests remain independent.
			lock.Lock()
			defer lock.Unlock()
		}
		return c.Next()
	}
}
