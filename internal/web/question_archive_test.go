package web

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/onlyin32bit/4t/internal/game"
	store "github.com/onlyin32bit/4t/internal/store/sqlite"
	_ "modernc.org/sqlite"
)

func TestCappedArchiveBufferRejectsOutputBeyondLimit(t *testing.T) {
	var buffer cappedArchiveBuffer
	buffer.limit = 3
	if n, err := buffer.Write([]byte("abcd")); n != 3 || err != errQuestionArchiveLimit {
		t.Fatalf("Write() = (%d, %v), want (3, limit error)", n, err)
	}
	if buffer.Len() != 3 {
		t.Fatalf("buffer length = %d, want 3", buffer.Len())
	}
}

func TestImportQuestionSetRollsBackAssetsWhenSaveFails(t *testing.T) {
	assetDir := filepath.Join(t.TempDir(), "assets")
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "4t.db")
	database, err := store.Open(ctx, databasePath)
	if err != nil {
		t.Fatal(err)
	}
	games, err := game.NewService(ctx, database, nil, nil)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}

	assetData := []byte("asset bytes")
	archive := archiveForImport(t, questionArchiveManifest{
		Format: questionArchiveFormat, Version: questionArchiveVersion,
		Name: "invalid question set", Data: json.RawMessage(`{"not_question_data":true}`),
		Assets: []questionArchiveAsset{{ID: "source", Path: "assets/source", OriginalName: "source.png", MediaType: "image/png", SHA256: digestForTest(assetData)}},
	}, map[string][]byte{"assets/source": assetData})
	response := importArchiveRequest(t, games, archive, assetDir)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("import status = %d, want 400", response.StatusCode)
	}
	response.Body.Close()

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if got := assetCount(t, databasePath); got != 0 {
		t.Fatalf("asset rows after failed import = %d, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(assetDir, digestForTest(assetData))); !os.IsNotExist(err) {
		t.Fatalf("imported asset file still exists after rollback: %v", err)
	}
}
func TestImportQuestionSetAcceptsLegacyRoundKeys(t *testing.T) {
	assetDir := filepath.Join(t.TempDir(), "assets")
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	games, err := game.NewService(ctx, database, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	legacy := json.RawMessage(`{"kd":{"chung":[{"content":"chung"}],"rieng":[[],[],[],[]]},"vcnv":{"obstacle":"Ô chữ","rows":[{},{},{},{}],"center_ques":"trung tâm"},"tt":[{"content":"tăng tốc"}],"vd":[{"20":[{"content":"A"}],"30":[{"content":"B"}]},{"20":[],"30":[]},{"20":[],"30":[]},{"20":[],"30":[]}]}`)
	archive := archiveForImport(t, questionArchiveManifest{Format: questionArchiveFormat, Version: questionArchiveVersion, Name: "legacy", Data: legacy}, nil)
	response := importArchiveRequest(t, games, archive, assetDir)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("legacy import status = %d", response.StatusCode)
	}
	response.Body.Close()
	sets, err := games.ListQuestionSets(ctx, "legacy")
	if err != nil || len(sets) != 1 {
		t.Fatalf("legacy question set was not saved: %v, %d", err, len(sets))
	}
	_, revision, err := games.GetQuestionSet(ctx, sets[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var normalized map[string]json.RawMessage
	if err := json.Unmarshal(revision.Data, &normalized); err != nil {
		t.Fatal(err)
	}
	if _, ok := normalized["kd_chung"]; !ok {
		t.Fatalf("legacy round keys were not normalized: %s", revision.Data)
	}
}

func TestImportQuestionSetPreservesExistingAssetOnFailure(t *testing.T) {
	assetDir := filepath.Join(t.TempDir(), "assets")
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "4t.db")
	database, err := store.Open(ctx, databasePath)
	if err != nil {
		t.Fatal(err)
	}
	games, err := game.NewService(ctx, database, nil, nil)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	assetData := []byte("existing asset")
	saved, err := persistAsset(ctx, games, game.Actor{}, "existing.png", "image/png", assetData, assetDir)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}

	archive := archiveForImport(t, questionArchiveManifest{
		Format: questionArchiveFormat, Version: questionArchiveVersion,
		Name: "invalid question set", Data: json.RawMessage(`{"not_question_data":true}`),
		Assets: []questionArchiveAsset{{ID: "source", Path: "assets/source", OriginalName: "source.png", MediaType: "image/png", SHA256: digestForTest(assetData)}},
	}, map[string][]byte{"assets/source": assetData})
	response := importArchiveRequest(t, games, archive, assetDir)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("import status = %d, want 400", response.StatusCode)
	}
	response.Body.Close()

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if got := assetCount(t, databasePath); got != 1 {
		t.Fatalf("asset rows after failed duplicate import = %d, want 1", got)
	}
	if _, err := os.Stat(filepath.Join(assetDir, saved.SHA256)); err != nil {
		t.Fatalf("existing asset file was removed: %v", err)
	}
}

func digestForTest(contents []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(contents))
}

func archiveForImport(t *testing.T, manifest questionArchiveManifest, entries map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestWriter, err := archive.Create("question-set.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifestWriter.Write(manifestData); err != nil {
		t.Fatal(err)
	}
	for path, data := range entries {
		writer, err := archive.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func importArchiveRequest(t *testing.T, games *game.Service, archive []byte, assetDir string) *http.Response {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "questions.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(archive); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Post("/", func(c *fiber.Ctx) error { return importQuestionSet(c, games, assetDir) })
	request, err := http.NewRequest(http.MethodPost, "/", &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func assetCount(t *testing.T, path string) int {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM assets").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
