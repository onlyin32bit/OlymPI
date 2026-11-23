package web

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/onlyin32bit/4t/internal/game"
)

const (
	questionArchiveFormat  = "4t-question-set"
	questionArchiveVersion = 1
	maxArchiveSize         = 16 * 1024 * 1024
	maxManifestSize        = 4 * 1024 * 1024
	maxAssetSize           = 12 * 1024 * 1024
	maxExportSize          = maxArchiveSize
)

var errQuestionArchiveLimit = errors.New("question-set archive exceeds size limit")
var errQuestionAssetLimit = errors.New("question-set asset exceeds size limit")

type cappedArchiveBuffer struct {
	bytes.Buffer
	limit int64
}

func (b *cappedArchiveBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - int64(b.Len())
	if remaining <= 0 {
		return 0, errQuestionArchiveLimit
	}
	if int64(len(p)) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		return int(remaining), errQuestionArchiveLimit
	}
	return b.Buffer.Write(p)
}

type questionArchiveManifest struct {
	Format   string                 `json:"format"`
	Version  int                    `json:"version"`
	Name     string                 `json:"name"`
	Category string                 `json:"category"`
	Data     json.RawMessage        `json:"data"`
	Assets   []questionArchiveAsset `json:"assets"`
}

type questionArchiveAsset struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	OriginalName string `json:"original_name"`
	MediaType    string `json:"media_type"`
	SHA256       string `json:"sha256"`
}

func exportQuestionSet(c *fiber.Ctx, games *game.Service, assetDir string) error {
	set, revision, err := games.GetQuestionSet(c.UserContext(), c.Params("id"))
	if err != nil {
		return gameHTTPError(c, err)
	}
	if int64(len(revision.Data)) > maxManifestSize {
		return fiber.NewError(413, "question-set manifest must not exceed 4 MB")
	}
	ids, err := referencedAssetIDs(revision.Data)
	if err != nil {
		return fiber.NewError(500, "question set contains invalid asset references")
	}

	manifest := questionArchiveManifest{
		Format: questionArchiveFormat, Version: questionArchiveVersion,
		Name: set.Name, Category: set.Category, Data: revision.Data,
		Assets: make([]questionArchiveAsset, 0, len(ids)),
	}
	var body cappedArchiveBuffer
	body.limit = maxExportSize
	archive := zip.NewWriter(&body)
	var totalAssetSize uint64
	for _, id := range ids {
		asset, assetErr := games.GetAsset(c.UserContext(), id)
		if assetErr != nil {
			_ = archive.Close()
			return fiber.NewError(409, "question set references a missing asset")
		}
		contents, readErr := readFileEntry(filepath.Join(assetDir, asset.SHA256), maxAssetSize)
		if readErr != nil {
			_ = archive.Close()
			if errors.Is(readErr, errQuestionAssetLimit) {
				return fiber.NewError(413, "question-set assets must not exceed 12 MB per asset")
			}
			return fiber.NewError(500, "cannot read a question-set asset")
		}
		totalAssetSize += uint64(len(contents))
		if totalAssetSize > maxArchiveSize {
			_ = archive.Close()
			return fiber.NewError(413, "question-set assets must not exceed 16 MB")
		}
		path := "assets/" + asset.ID
		writer, createErr := archive.Create(path)
		if createErr != nil {
			_ = archive.Close()
			if errors.Is(createErr, errQuestionArchiveLimit) {
				return fiber.NewError(413, "question-set archive must not exceed 16 MB")
			}
			return fiber.NewError(500, "cannot create question-set archive")
		}
		if _, writeErr := writer.Write(contents); writeErr != nil {
			_ = archive.Close()
			if errors.Is(writeErr, errQuestionArchiveLimit) {
				return fiber.NewError(413, "question-set archive must not exceed 16 MB")
			}
			return fiber.NewError(500, "cannot write question-set archive")
		}
		manifest.Assets = append(manifest.Assets, questionArchiveAsset{
			ID: asset.ID, Path: path, OriginalName: asset.OriginalName,
			MediaType: asset.MediaType, SHA256: asset.SHA256,
		})
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = archive.Close()
		return fiber.NewError(500, "cannot encode question-set archive")
	}
	if len(manifestData) > maxManifestSize {
		_ = archive.Close()
		return fiber.NewError(413, "question-set manifest must not exceed 4 MB")
	}
	manifestWriter, err := archive.Create("question-set.json")
	if err != nil {
		_ = archive.Close()
		if errors.Is(err, errQuestionArchiveLimit) {
			return fiber.NewError(413, "question-set archive must not exceed 16 MB")
		}
		return fiber.NewError(500, "cannot create question-set archive")
	}
	if _, err = manifestWriter.Write(manifestData); err != nil {
		_ = archive.Close()
		if errors.Is(err, errQuestionArchiveLimit) {
			return fiber.NewError(413, "question-set archive must not exceed 16 MB")
		}
		return fiber.NewError(500, "cannot write question-set archive")
	}
	if err = archive.Close(); err != nil {
		if errors.Is(err, errQuestionArchiveLimit) {
			return fiber.NewError(413, "question-set archive must not exceed 16 MB")
		}
		return fiber.NewError(500, "cannot finish question-set archive")
	}

	_ = games.RecordAudit(c.UserContext(), game.AuditEntry{
		Action: "question_set.export", EntityType: "question_set", EntityID: set.ID,
	}, httpActor(c))
	c.Set(fiber.HeaderContentType, "application/zip")
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="question-set-%s.zip"`, set.ID))
	return c.Send(body.Bytes())
}

func importQuestionSet(c *fiber.Ctx, games *game.Service, assetDir string) error {
	header, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(400, "ZIP file is required")
	}
	if header.Size <= 0 || header.Size > maxArchiveSize {
		return fiber.NewError(413, "question-set archive must not exceed 16 MB")
	}
	file, err := header.Open()
	if err != nil {
		return fiber.NewError(400, "cannot read question-set archive")
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, maxArchiveSize+1))
	if err != nil || len(contents) > maxArchiveSize {
		return fiber.NewError(413, "question-set archive must not exceed 16 MB")
	}
	reader, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		return fiber.NewError(400, "file is not a valid ZIP archive")
	}
	files := make(map[string]*zip.File, len(reader.File))
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if _, exists := files[entry.Name]; exists {
			return fiber.NewError(400, "archive contains duplicate paths")
		}
		files[entry.Name] = entry
	}
	manifestFile := files["question-set.json"]
	if manifestFile == nil {
		return fiber.NewError(400, "archive is missing question-set.json")
	}
	manifestData, err := readZipEntry(manifestFile, maxManifestSize)
	if err != nil {
		return fiber.NewError(400, "cannot read question-set manifest")
	}
	var manifest questionArchiveManifest
	if err = json.Unmarshal(manifestData, &manifest); err != nil || manifest.Format != questionArchiveFormat || manifest.Version != questionArchiveVersion {
		return fiber.NewError(400, "unsupported question-set archive")
	}

	remap := make(map[string]string, len(manifest.Assets))
	imported := make([]persistedAsset, 0, len(manifest.Assets))
	var totalAssetSize uint64
	for _, archived := range manifest.Assets {
		if archived.ID == "" || archived.Path == "" || remap[archived.ID] != "" || !allowedMedia(archived.MediaType) {
			cleanupPersistedAssets(c.UserContext(), games, imported)
			return fiber.NewError(400, "archive contains invalid asset metadata")
		}
		entry := files[archived.Path]
		if entry == nil {
			cleanupPersistedAssets(c.UserContext(), games, imported)
			return fiber.NewError(400, "archive is missing a referenced asset")
		}
		totalAssetSize += entry.UncompressedSize64
		if totalAssetSize > maxArchiveSize {
			cleanupPersistedAssets(c.UserContext(), games, imported)
			return fiber.NewError(413, "question-set assets must not exceed 16 MB")
		}
		assetData, readErr := readZipEntry(entry, maxAssetSize)
		if readErr != nil {
			cleanupPersistedAssets(c.UserContext(), games, imported)
			return fiber.NewError(400, "archive asset is invalid or too large")
		}
		digest := fmt.Sprintf("%x", sha256.Sum256(assetData))
		if archived.SHA256 != "" && !strings.EqualFold(archived.SHA256, digest) {
			cleanupPersistedAssets(c.UserContext(), games, imported)
			return fiber.NewError(400, "archive asset hash does not match its manifest")
		}
		asset, saveState, saveErr := persistAssetTracked(c.UserContext(), games, httpActor(c), archived.OriginalName, archived.MediaType, assetData, assetDir)
		if saveErr != nil {
			cleanupPersistedAssets(c.UserContext(), games, imported)
			return gameHTTPError(c, saveErr)
		}
		imported = append(imported, saveState)
		remap[archived.ID] = asset.ID
	}
	data, err := remapQuestionAssets(manifest.Data, remap)
	if err != nil {
		cleanupPersistedAssets(c.UserContext(), games, imported)
		return fiber.NewError(400, err.Error())
	}
	created, err := games.SaveQuestionSet(c.UserContext(), "", game.SaveQuestionSetInput{
		Name: manifest.Name, Category: manifest.Category, Data: data,
	}, httpActor(c))
	if err != nil {
		cleanupPersistedAssets(c.UserContext(), games, imported)
		return gameHTTPError(c, err)
	}
	_ = games.RecordAudit(c.UserContext(), game.AuditEntry{
		Action: "question_set.import", EntityType: "question_set", EntityID: created.ID,
	}, httpActor(c))
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"question_set": created})
}

type persistedAsset struct {
	id         string
	path       string
	deleteRow  bool
	removeFile bool
}

func cleanupPersistedAssets(ctx context.Context, games *game.Service, assets []persistedAsset) {
	for i := len(assets) - 1; i >= 0; i-- {
		asset := assets[i]
		deleted := true
		if asset.deleteRow {
			deleted = games.DeleteAsset(ctx, asset.id) == nil
		}
		if asset.removeFile && deleted {
			_ = os.Remove(asset.path)
		}
	}
}

func persistAsset(ctx context.Context, games *game.Service, actor game.Actor, originalName, mediaType string, contents []byte, assetDir string) (game.Asset, error) {
	asset, _, err := persistAssetTracked(ctx, games, actor, originalName, mediaType, contents, assetDir)
	return asset, err
}

func persistAssetTracked(ctx context.Context, games *game.Service, actor game.Actor, originalName, mediaType string, contents []byte, assetDir string) (game.Asset, persistedAsset, error) {
	digest := fmt.Sprintf("%x", sha256.Sum256(contents))
	if err := os.MkdirAll(assetDir, 0o700); err != nil {
		return game.Asset{}, persistedAsset{}, err
	}
	path := filepath.Join(assetDir, digest)
	_, statErr := os.Stat(path)
	fileExisted := statErr == nil || !os.IsNotExist(statErr)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		return game.Asset{}, persistedAsset{}, err
	}
	if originalName == "" {
		originalName = "asset"
	}
	id := uuid.NewString()
	asset, err := games.SaveAsset(ctx, game.Asset{
		ID: id, SHA256: digest, OriginalName: filepath.Base(originalName),
		MediaType: mediaType, Size: int64(len(contents)),
	}, actor)
	if err != nil {
		if !fileExisted {
			_ = os.Remove(path)
		}
		_ = games.DeleteAsset(ctx, id)
		return game.Asset{}, persistedAsset{}, err
	}
	return asset, persistedAsset{
		id: asset.ID, path: path,
		deleteRow:  asset.ID == id,
		removeFile: asset.ID == id && !fileExisted,
	}, nil
}

func referencedAssetIDs(raw json.RawMessage) ([]string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	ids := map[string]struct{}{}
	walkQuestionData(value, func(key string, current *any) error {
		if !isAssetKey(key) {
			return nil
		}
		id, ok := (*current).(string)
		if ok && id != "" {
			ids[id] = struct{}{}
		}
		return nil
	})
	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Strings(result)
	return result, nil
}

func remapQuestionAssets(raw json.RawMessage, remap map[string]string) (json.RawMessage, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, errors.New("question-set manifest contains invalid data")
	}
	err := walkQuestionData(value, func(key string, current *any) error {
		if !isAssetKey(key) {
			return nil
		}
		id, ok := (*current).(string)
		if !ok || id == "" {
			return nil
		}
		mapped, exists := remap[id]
		if !exists {
			return fmt.Errorf("archive does not contain referenced asset %s", id)
		}
		*current = mapped
		return nil
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func walkQuestionData(value any, visit func(string, *any) error) error {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if err := visit(key, &child); err != nil {
				return err
			}
			current[key] = child
			if err := walkQuestionData(child, visit); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range current {
			if err := walkQuestionData(child, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

func isAssetKey(key string) bool {
	return key == "asset_id" || key == "obstacle_asset_id" || key == "explanation_asset_id"
}

func readFileEntry(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(contents)) > limit {
		return nil, errQuestionAssetLimit
	}
	return contents, nil
}

func readZipEntry(file *zip.File, limit int64) ([]byte, error) {
	if file.UncompressedSize64 > uint64(limit) {
		return nil, errors.New("ZIP entry is too large")
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	contents, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil || int64(len(contents)) > limit {
		return nil, errors.New("ZIP entry is too large")
	}
	return contents, nil
}
