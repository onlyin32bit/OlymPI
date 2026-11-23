package web

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/sound"
)

const maxSoundUpload = 12 * 1024 * 1024

func registerPublicSoundRoutes(api fiber.Router, sounds *sound.Service) {
	api.Get("/sounds", func(c *fiber.Ctx) error {
		values, err := sounds.List(c.UserContext(), false)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to load sounds")
		}
		return c.JSON(fiber.Map{"sounds": values})
	})
}

func registerSoundRoutes(private fiber.Router, sounds *sound.Service, games *game.Service, assetDir string) {
	admin := private.Group("", requireRole(auth.RoleAdmin))
	admin.Get("/sounds/manage", func(c *fiber.Ctx) error {
		values, err := sounds.List(c.UserContext(), true)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to load sounds")
		}
		return c.JSON(fiber.Map{"sounds": values})
	})
	admin.Post("/sounds", func(c *fiber.Ctx) error {
		header, err := c.FormFile("file")
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "audio file is required")
		}
		if header.Size > maxSoundUpload {
			return fiber.NewError(fiber.StatusRequestEntityTooLarge, "audio file must not exceed 12 MB")
		}
		file, err := header.Open()
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "cannot read audio file")
		}
		defer file.Close()
		contents, err := io.ReadAll(io.LimitReader(file, maxSoundUpload+1))
		if err != nil || len(contents) > maxSoundUpload {
			return fiber.NewError(fiber.StatusRequestEntityTooLarge, "audio file must not exceed 12 MB")
		}
		mediaType := http.DetectContentType(contents)
		if mediaType == "application/octet-stream" && strings.EqualFold(filepath.Ext(header.Filename), ".m4a") {
			mediaType = "audio/mp4"
		}
		if !allowedSoundMedia(mediaType) {
			return fiber.NewError(fiber.StatusBadRequest, "only MP3, WAV, OGG, and M4A audio files are supported")
		}
		var metadata sound.Update
		if err := json.Unmarshal([]byte(c.FormValue("metadata")), &metadata); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid sound metadata")
		}
		if strings.TrimSpace(metadata.Name) == "" {
			metadata.Name = strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
		}
		asset, persisted, err := persistAssetTracked(c.UserContext(), games, httpActor(c), header.Filename, mediaType, contents, assetDir)
		if err != nil {
			return gameHTTPError(c, err)
		}
		value, err := sounds.Create(c.UserContext(), asset.ID, metadata, httpActor(c))
		if err != nil {
			cleanupPersistedAssets(c.UserContext(), games, []persistedAsset{persisted})
			return gameHTTPError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"sound": value})
	})
	admin.Put("/sounds/:id", func(c *fiber.Ctx) error {
		var update sound.Update
		if err := c.BodyParser(&update); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid sound metadata")
		}
		value, err := sounds.Update(c.UserContext(), c.Params("id"), update, httpActor(c))
		if err != nil {
			return gameHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"sound": value})
	})
	admin.Delete("/sounds/:id", func(c *fiber.Ctx) error {
		if err := sounds.Archive(c.UserContext(), c.Params("id"), httpActor(c)); err != nil {
			return gameHTTPError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
}

func allowedSoundMedia(value string) bool {
	switch value {
	case "audio/mpeg", "audio/wav", "audio/x-wav", "audio/wave", "audio/ogg", "application/ogg", "audio/mp4":
		return true
	default:
		return false
	}
}
