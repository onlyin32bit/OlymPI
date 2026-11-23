package web

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

func deleteSessionCookie(c *fiber.Ctx, secure bool) {
	c.Cookie(&fiber.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", HTTPOnly: true,
		// fasthttp omits negative MaxAge, so explicitly expire the cookie.
		Expires: time.Unix(1, 0).UTC(),
		Secure:  secure, SameSite: fiber.CookieSameSiteLaxMode,
	})
}
