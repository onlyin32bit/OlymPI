package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
)

func registerPublicGameRoutes(api fiber.Router, games *game.Service, assetDir string) {
	api.Get("/match/current", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"match": games.Current(opsocket.Principal{})})
	})
	api.Get("/match/current/vcnv", func(c *fiber.Ctx) error {
		value, err := games.CurrentVCNV(c.UserContext())
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"vcnv": value})
	})
	api.Get("/assets/:id", func(c *fiber.Ctx) error {
		asset, err := games.GetAsset(c.UserContext(), c.Params("id"))
		if err != nil {
			return fiber.NewError(404, "asset not found")
		}
		c.Set(fiber.HeaderContentType, asset.MediaType)
		c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
		return c.SendFile(filepath.Join(assetDir, asset.SHA256), false)
	})
}

func registerGameRoutes(private fiber.Router, games *game.Service, sessions *auth.Service, socket *opsocket.Socket, assetDir string) {
	admin := private.Group("", requireRole(auth.RoleAdmin))
	admin.Get("/matches", func(c *fiber.Ctx) error {
		values, err := games.ListMatches(c.UserContext(), c.Query("q"))
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"matches": values})
	})
	admin.Post("/matches", func(c *fiber.Ctx) error {
		var input game.CreateMatchInput
		if err := c.BodyParser(&input); err != nil {
			return fiber.NewError(400, "invalid match payload")
		}
		value, err := games.CreateMatch(c.UserContext(), input, httpActor(c))
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"match": value})
	})
	admin.Get("/matches/:id", func(c *fiber.Ctx) error {
		value, err := games.GetMatch(c.UserContext(), c.Params("id"))
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"match": value})
	})
	admin.Put("/matches/:id", func(c *fiber.Ctx) error {
		var input game.UpdateMatchInput
		if err := c.BodyParser(&input); err != nil {
			return fiber.NewError(400, "invalid match payload")
		}
		value, err := games.UpdateMatch(c.UserContext(), c.Params("id"), input, httpActor(c))
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"match": value})
	})
	admin.Delete("/matches/:id", func(c *fiber.Ctx) error {
		version, _ := strconv.ParseInt(c.Query("version"), 10, 64)
		if err := games.ArchiveMatch(c.UserContext(), c.Params("id"), version, httpActor(c)); err != nil {
			return gameHTTPError(c, err)
		}
		return c.SendStatus(204)
	})
	admin.Post("/matches/:id/duplicate", func(c *fiber.Ctx) error {
		value, err := games.DuplicateMatch(c.UserContext(), c.Params("id"), httpActor(c))
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"match": value})
	})
	admin.Post("/matches/:id/select", func(c *fiber.Ctx) error {
		value, err := games.SelectMatch(c.UserContext(), c.Params("id"), httpActor(c))
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"runtime": value})
	})
	admin.Get("/matches/:id/events", func(c *fiber.Ctx) error {
		var afterSequence int64
		if c.Context().QueryArgs().Has("after_sequence") {
			value, err := strconv.ParseInt(c.Query("after_sequence"), 10, 64)
			if err != nil || value < 0 {
				return fiber.NewError(http.StatusBadRequest, "after_sequence must be a non-negative integer")
			}
			afterSequence = value
		}
		values, err := games.ListEventsAfter(c.UserContext(), c.Params("id"), afterSequence)
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"events": values})
	})
	admin.Post("/matches/:id/amendments", func(c *fiber.Ctx) error {
		var body struct {
			Result json.RawMessage `json:"result"`
			Reason string          `json:"reason"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(400, "invalid amendment payload")
		}
		if err := games.AmendResult(c.UserContext(), c.Params("id"), body.Result, body.Reason, httpActor(c)); err != nil {
			return gameHTTPError(c, err)
		}
		return c.SendStatus(204)
	})

	admin.Get("/question-sets", func(c *fiber.Ctx) error {
		values, err := games.ListQuestionSets(c.UserContext(), c.Query("q"))
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"question_sets": values})
	})
	admin.Post("/question-sets", func(c *fiber.Ctx) error {
		var input game.SaveQuestionSetInput
		if err := c.BodyParser(&input); err != nil {
			return fiber.NewError(400, "invalid question set payload")
		}
		value, err := games.SaveQuestionSet(c.UserContext(), "", input, httpActor(c))
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"question_set": value})
	})
	admin.Post("/question-sets/import", func(c *fiber.Ctx) error {
		return importQuestionSet(c, games, assetDir)
	})
	admin.Get("/question-sets/:id/export", func(c *fiber.Ctx) error {
		return exportQuestionSet(c, games, assetDir)
	})
	admin.Get("/question-sets/:id", func(c *fiber.Ctx) error {
		value, revision, err := games.GetQuestionSet(c.UserContext(), c.Params("id"))
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"question_set": value, "revision": revision})
	})
	admin.Get("/question-set-revisions/:id", func(c *fiber.Ctx) error {
		revision, err := games.GetQuestionSetRevision(c.UserContext(), c.Params("id"))
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"revision": revision})
	})
	admin.Put("/question-sets/:id", func(c *fiber.Ctx) error {
		var input game.SaveQuestionSetInput
		if err := c.BodyParser(&input); err != nil {
			return fiber.NewError(400, "invalid question set payload")
		}
		value, err := games.SaveQuestionSet(c.UserContext(), c.Params("id"), input, httpActor(c))
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"question_set": value})
	})
	admin.Delete("/question-sets/:id", func(c *fiber.Ctx) error {
		version, _ := strconv.ParseInt(c.Query("version"), 10, 64)
		if err := games.ArchiveQuestionSet(c.UserContext(), c.Params("id"), version, httpActor(c)); err != nil {
			return gameHTTPError(c, err)
		}
		return c.SendStatus(204)
	})

	admin.Get("/audit-logs", func(c *fiber.Ctx) error {
		limit, _ := strconv.Atoi(c.Query("limit"))
		values, err := games.ListAudit(c.UserContext(), c.Query("q"), limit)
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"audit_logs": values})
	})
	admin.Get("/contestant-accounts", func(c *fiber.Ctx) error {
		values, err := sessions.ListFixedContestants(c.UserContext())
		if err != nil {
			return err
		}
		return c.JSON(fiber.Map{"accounts": values})
	})
	admin.Put("/contestant-accounts/:slot/password", func(c *fiber.Ctx) error {
		slot, _ := strconv.Atoi(c.Params("slot"))
		var body struct {
			Password string `json:"password"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(400, "invalid password payload")
		}
		if err := sessions.ResetFixedContestantPassword(c.UserContext(), slot, body.Password); err != nil {
			return fiber.NewError(400, err.Error())
		}
		accounts, _ := sessions.ListFixedContestants(c.UserContext())
		for _, account := range accounts {
			if account.FixedSlot == slot {
				socket.DisconnectSubject(strconv.FormatInt(account.ID, 10))
				break
			}
		}
		if err := games.RecordAudit(c.UserContext(), game.AuditEntry{Action: "contestant.password_reset", EntityType: "user", EntityID: strconv.Itoa(slot)}, httpActor(c)); err != nil {
			return fiber.NewError(500, "password was reset but the audit record could not be written")
		}
		return c.SendStatus(204)
	})
	admin.Get("/admin-accounts", func(c *fiber.Ctx) error {
		values, err := sessions.ListAdmins(c.UserContext())
		if err != nil {
			return err
		}
		return c.JSON(fiber.Map{"accounts": values})
	})
	admin.Post("/admin-accounts", func(c *fiber.Ctx) error {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(400, "invalid account payload")
		}
		created, err := sessions.CreateAdmin(c.UserContext(), body.Username, body.Password)
		if err != nil {
			return fiber.NewError(400, err.Error())
		}
		if err := games.RecordAudit(c.UserContext(), game.AuditEntry{Action: "admin.create", EntityType: "user", EntityID: strconv.FormatInt(created.ID, 10)}, httpActor(c)); err != nil {
			return fiber.NewError(500, "account was created but the audit record could not be written")
		}
		return c.Status(201).JSON(fiber.Map{"account": created})
	})
	admin.Put("/admin-accounts/:id/password", func(c *fiber.Ctx) error {
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil {
			return fiber.NewError(400, "invalid account id")
		}
		var body struct {
			Password string `json:"password"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(400, "invalid password payload")
		}
		if err := sessions.ResetAdminPassword(c.UserContext(), id, body.Password); err != nil {
			return fiber.NewError(400, err.Error())
		}
		socket.DisconnectSubject(strconv.FormatInt(id, 10))
		if err := games.RecordAudit(c.UserContext(), game.AuditEntry{Action: "admin.password_reset", EntityType: "user", EntityID: strconv.FormatInt(id, 10)}, httpActor(c)); err != nil {
			return fiber.NewError(500, "password was reset but the audit record could not be written")
		}
		return c.SendStatus(204)
	})
	admin.Put("/admin-accounts/:id/enabled", func(c *fiber.Ctx) error {
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil {
			return fiber.NewError(400, "invalid account id")
		}
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(400, "invalid account payload")
		}
		actor, _ := c.Locals("user").(auth.UserDTO)
		if err := sessions.SetAdminEnabled(c.UserContext(), id, actor.ID, body.Enabled); err != nil {
			return fiber.NewError(400, err.Error())
		}
		if !body.Enabled {
			socket.DisconnectSubject(strconv.FormatInt(id, 10))
		}
		action := "admin.disable"
		if body.Enabled {
			action = "admin.enable"
		}
		if err := games.RecordAudit(c.UserContext(), game.AuditEntry{Action: action, EntityType: "user", EntityID: strconv.FormatInt(id, 10)}, httpActor(c)); err != nil {
			return fiber.NewError(500, "account changed but the audit record could not be written")
		}
		return c.SendStatus(204)
	})
	admin.Get("/sessions", func(c *fiber.Ctx) error {
		values, err := sessions.ListSessions(c.UserContext(), c.Cookies(sessionCookieName))
		if err != nil {
			return err
		}
		return c.JSON(fiber.Map{"sessions": values})
	})
	admin.Delete("/sessions/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		if err := sessions.InvalidateSession(c.UserContext(), id); err != nil {
			return fiber.NewError(400, err.Error())
		}
		socket.DisconnectSession(id)
		if err := games.RecordAudit(c.UserContext(), game.AuditEntry{Action: "session.invalidate", EntityType: "session", EntityID: id}, httpActor(c)); err != nil {
			return fiber.NewError(500, "session was invalidated but the audit record could not be written")
		}
		return c.SendStatus(204)
	})
	admin.Post("/assets", func(c *fiber.Ctx) error {
		header, err := c.FormFile("file")
		if err != nil {
			return fiber.NewError(400, "file is required")
		}
		if header.Size > 12*1024*1024 {
			return fiber.NewError(413, "file must not exceed 12 MB")
		}
		file, err := header.Open()
		if err != nil {
			return fiber.NewError(400, "cannot read file")
		}
		defer file.Close()
		contents, err := io.ReadAll(io.LimitReader(file, 12*1024*1024+1))
		if err != nil || len(contents) > 12*1024*1024 {
			return fiber.NewError(413, "file must not exceed 12 MB")
		}
		mediaType := http.DetectContentType(contents)
		if !allowedMedia(mediaType) {
			return fiber.NewError(400, "only PNG, JPEG, WebP, MP3, WAV, OGG, and MP4 are supported")
		}
		asset, err := persistAsset(c.UserContext(), games, httpActor(c), header.Filename, mediaType, contents, assetDir)
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"asset": asset})
	})
}

func allowedMedia(value string) bool {
	switch value {
	case "image/png", "image/jpeg", "image/webp", "audio/mpeg", "audio/wav", "audio/x-wav", "audio/wave", "audio/ogg", "application/ogg", "video/mp4":
		return true
	default:
		return false
	}
}

func httpActor(c *fiber.Ctx) game.Actor {
	user, _ := c.Locals("user").(auth.UserDTO)
	return game.Actor{UserID: user.ID, ClientID: c.IP()}
}

func gameHTTPError(c *fiber.Ctx, err error) error {
	var operation *opsocket.Error
	if !errors.As(err, &operation) {
		return fiber.NewError(500, "operation failed")
	}
	status := fiber.StatusBadRequest
	switch operation.Code {
	case "NOT_FOUND":
		status = 404
	case "FORBIDDEN":
		status = 403
	case "CONFLICT", "STALE_RUNTIME", "MATCH_PLAYING":
		status = 409
	case "INTERNAL":
		status = 500
	}
	return c.Status(status).JSON(operation)
}
