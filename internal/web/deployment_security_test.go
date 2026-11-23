package web

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/config"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/sound"
	store "github.com/onlyin32bit/4t/internal/store/sqlite"
)

func TestRequestHostPolicyRequiresIndependentTrust(t *testing.T) {
	localIPs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("192.0.2.10"), net.ParseIP("2001:db8::10")}
	for _, test := range []struct {
		name              string
		args              []string
		allowed, rejected []string
	}{
		{
			name:     "default loopback",
			allowed:  []string{"localhost:8090", "LOCALHOST:5173", "127.0.0.1:8090", "127.0.0.2", "[::1]:8090"},
			rejected: []string{"attacker.example:8090", "localhost.attacker.example:8090", "192.0.2.10:8090", "evil@localhost:8090", "localhost:0", "localhost:65536", "localhost/path", "::1"},
		},
		{
			name:     "wildcard LAN TLS",
			args:     []string{"--bind", "::", "--tls-cert", "cert.pem", "--tls-key", "key.pem"},
			allowed:  []string{"localhost:8090", "192.0.2.10:8090", "[2001:db8::10]:8090"},
			rejected: []string{"attacker.example:8090", "192.0.2.11:8090", "[2001:db8::11]:8090", "0.0.0.0:8090", "[::]:8090"},
		},
		{
			name:     "explicit named TLS host",
			args:     []string{"--bind", "192.0.2.10", "--tls-cert", "cert.pem", "--tls-key", "key.pem", "--allowed-host", "game.example"},
			allowed:  []string{"game.example:8090", "GAME.EXAMPLE.:8090", "192.0.2.10:8090"},
			rejected: []string{"attacker.example:8090", "game.example.attacker.example:8090", "[2001:db8::10]:8090"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := config.Load(test.args)
			if err != nil {
				t.Fatal(err)
			}
			policy := newRequestHostPolicy(cfg, localIPs)
			for _, host := range test.allowed {
				if !policy.allows(host) {
					t.Errorf("trusted Host %q rejected", host)
				}
			}
			for _, host := range test.rejected {
				if policy.allows(host) {
					t.Errorf("untrusted Host %q accepted", host)
				}
			}
		})
	}
}

func TestFreshLoopbackSetupRejectsMatchingReboundHostAndOrigin(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	sessions := auth.NewService(database)
	system, err := config.OpenSystemSettings(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Use(requireTrustedHost(cfg))
	registerSetupRoutes(app.Group("/api"), sessions, system)
	request := mustRequest(t, http.MethodPost, "http://attacker.example:8090/api/setup", strings.NewReader(`{"username":"attacker","password":"password123","bind":"127.0.0.1","port":8090}`))
	request.Header.Set("Origin", "http://attacker.example:8090")
	request.Header.Set("X-Forwarded-Host", "localhost:8090")
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("rebound setup status = %d, want 403", response.StatusCode)
	}
	if hasAdmin, err := sessions.HasAdmin(ctx); err != nil || hasAdmin || system.Configured() {
		t.Fatalf("rebound setup changed initial state: hasAdmin=%v configured=%v err=%v", hasAdmin, system.Configured(), err)
	}
}

func TestServerRejectsUntrustedHostBeforeAPIAndWebsocket(t *testing.T) {
	server, _, _ := deploymentServer(t)
	for _, target := range []string{"/api/setup/status", "/ws"} {
		request := mustRequest(t, http.MethodGet, "https://attacker.example:8443"+target, nil)
		request.Header.Set("Origin", "https://attacker.example:8443")
		response, err := server.app.Test(request, -1)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("untrusted Host reached %s: status %d", target, response.StatusCode)
		}
	}
}

func mediaQuestionData(assetID string) json.RawMessage {
	return json.RawMessage(`{"kd_chung":[],"kd_rieng":[[],[],[],[]],"vcnv":{"obstacle":"A","obstacle_asset_id":"` + assetID + `","rows":[{},{},{},{}],"center":{}},"tt":[],"vd":[]}`)
}

func TestWAVAndOGGUploadsAndArchiveRoundTrip(t *testing.T) {
	wav := make([]byte, 46)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 8000)
	binary.LittleEndian.PutUint32(wav[28:], 8000)
	binary.LittleEndian.PutUint16(wav[32:], 1)
	binary.LittleEndian.PutUint16(wav[34:], 8)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], 2)
	wav[44], wav[45] = 128, 128
	ogg, err := os.ReadFile(filepath.Join("..", "..", "ui", "static", "sounds", "tt", "AnswersShowing.ogg"))
	if err != nil {
		t.Fatal(err)
	}
	server, games, _ := deploymentServer(t)
	cookie := deploymentLogin(t, server)
	otherServer, _, otherDataDir := deploymentServer(t)
	otherCookie := deploymentLogin(t, otherServer)
	for _, test := range []struct {
		name, mediaType string
		contents        []byte
	}{
		{"sound.wav", "audio/wave", wav},
		{"sound.ogg", "application/ogg", ogg},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := http.DetectContentType(test.contents); got != test.mediaType {
				t.Fatalf("fixture MIME = %q, want %q", got, test.mediaType)
			}
			var uploaded struct {
				Asset game.Asset `json:"asset"`
			}
			if err := json.Unmarshal(deploymentUpload(t, server, cookie, "/api/assets", test.name, test.contents, ""), &uploaded); err != nil {
				t.Fatal(err)
			}
			if uploaded.Asset.MediaType != test.mediaType {
				t.Fatalf("asset MIME = %q", uploaded.Asset.MediaType)
			}
			var audio struct {
				Sound sound.Sound `json:"sound"`
			}
			if err := json.Unmarshal(deploymentUpload(t, server, cookie, "/api/sounds", test.name, test.contents, `{"name":"actual audio","category":"test","bus":"effects","volume":1,"enabled":true}`), &audio); err != nil {
				t.Fatal(err)
			}
			asset, err := games.GetAsset(context.Background(), audio.Sound.AssetID)
			if err != nil || asset.MediaType != test.mediaType {
				t.Fatalf("sound MIME = %q, err=%v", asset.MediaType, err)
			}
			set, err := games.SaveQuestionSet(context.Background(), "", game.SaveQuestionSetInput{Name: test.name, Data: mediaQuestionData(uploaded.Asset.ID)}, game.Actor{})
			if err != nil {
				t.Fatal(err)
			}
			archive := deploymentRequest(t, server, http.MethodGet, "/api/question-sets/"+set.ID+"/export", nil, "", cookie, http.StatusOK)
			deploymentUpload(t, otherServer, otherCookie, "/api/question-sets/import", "questions.zip", archive, "")
			contents, err := os.ReadFile(filepath.Join(otherDataDir, "assets", uploaded.Asset.SHA256))
			if err != nil || !bytes.Equal(contents, test.contents) {
				t.Fatalf("audio archive bytes changed: err=%v", err)
			}
		})
	}
}

type pausedPublicationRepository struct {
	game.Repository
	calls                 atomic.Int32
	rolledBack            atomic.Bool
	adoptedBeforeRollback atomic.Bool
	published             chan game.Asset
	release               chan struct{}
}

func (r *pausedPublicationRepository) SaveAsset(ctx context.Context, asset game.Asset, actor game.Actor) (game.Asset, error) {
	first := r.calls.Add(1) == 1
	saved, err := r.Repository.SaveAsset(ctx, asset, actor)
	if err != nil {
		return saved, err
	}
	if first {
		r.published <- saved
		<-r.release
	} else if !r.rolledBack.Load() {
		r.adoptedBeforeRollback.Store(true)
	}
	return saved, nil
}

func (r *pausedPublicationRepository) DeleteAsset(ctx context.Context, id string) error {
	err := r.Repository.DeleteAsset(ctx, id)
	if err == nil {
		r.rolledBack.Store(true)
	}
	return err
}

type deploymentHTTPResult struct {
	status int
	body   []byte
	err    error
}

func asyncDeploymentRequest(server *Server, request *http.Request) <-chan deploymentHTTPResult {
	result := make(chan deploymentHTTPResult, 1)
	go func() {
		response, err := server.app.Test(request, -1)
		if err != nil {
			result <- deploymentHTTPResult{err: err}
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		result <- deploymentHTTPResult{status: response.StatusCode, body: body, err: err}
	}()
	return result
}

func waitDeploymentResult(t *testing.T, result <-chan deploymentHTTPResult) deploymentHTTPResult {
	t.Helper()
	select {
	case response := <-result:
		if response.err != nil {
			t.Fatal(response.err)
		}
		return response
	case <-time.After(5 * time.Second):
		t.Fatal("request blocked past the publication barrier deadline")
		return deploymentHTTPResult{}
	}
}

func publicationRequest(t *testing.T, path, filename string, contents []byte, metadata string, cookie *http.Cookie) *http.Request {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(contents); err != nil {
		t.Fatal(err)
	}
	if metadata != "" {
		if err := form.WriteField("metadata", metadata); err != nil {
			t.Fatal(err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	request := mustRequest(t, http.MethodPost, "https://192.0.2.10:8443"+path, &body)
	request.Header.Set("Origin", "https://192.0.2.10:8443")
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.AddCookie(cookie)
	return request
}

func TestFailedImportCannotRollBackConcurrentAssetAdoption(t *testing.T) {
	for _, secondPath := range []string{"/api/question-sets/import", "/api/assets", "/api/sounds"} {
		t.Run(secondPath, func(t *testing.T) {
			repository := &pausedPublicationRepository{published: make(chan game.Asset, 1), release: make(chan struct{})}
			server, games, dataDir := deploymentServerWithRepository(t, func(base game.Repository) game.Repository { repository.Repository = base; return repository })
			cookie := deploymentLogin(t, server)
			assetData := []byte("ID3\x03\x00\x00\x00\x00\x00\x00shared-publication-audio")
			manifest := questionArchiveManifest{
				Format: questionArchiveFormat, Version: questionArchiveVersion, Name: "invalid",
				Data:   json.RawMessage(`{"not_question_data":true}`),
				Assets: []questionArchiveAsset{{ID: "source", Path: "assets/source", OriginalName: "shared.mp3", MediaType: "audio/mpeg", SHA256: digestForTest(assetData)}},
			}
			failedArchive := archiveForImport(t, manifest, map[string][]byte{"assets/source": assetData})
			firstResult := asyncDeploymentRequest(server, publicationRequest(t, "/api/question-sets/import", "invalid.zip", failedArchive, "", cookie))
			released := false
			defer func() {
				if !released {
					close(repository.release)
				}
			}()
			var firstAsset game.Asset
			select {
			case firstAsset = <-repository.published:
			case <-time.After(5 * time.Second):
				t.Fatal("first import did not reach publication barrier")
			}
			if server.assetWrites.TryLock() {
				server.assetWrites.Unlock()
				t.Fatal("import published an asset outside the commit/rollback critical section")
			}
			read := asyncDeploymentRequest(server, mustRequest(t, http.MethodGet, "https://192.0.2.10:8443/api/setup/status", nil))
			if response := waitDeploymentResult(t, read); response.status != http.StatusOK {
				t.Fatalf("read blocked/rejected while import pending: %d", response.status)
			}
			secondData, filename, metadata := assetData, "shared.mp3", ""
			if secondPath == "/api/question-sets/import" {
				manifest.Name, manifest.Data = "valid", mediaQuestionData("source")
				secondData = archiveForImport(t, manifest, map[string][]byte{"assets/source": assetData})
				filename = "valid.zip"
			} else if secondPath == "/api/sounds" {
				metadata = `{"name":"shared audio","category":"test","bus":"effects","volume":1,"enabled":true}`
			}
			secondResult := asyncDeploymentRequest(server, publicationRequest(t, secondPath, filename, secondData, metadata, cookie))
			close(repository.release)
			released = true
			if response := waitDeploymentResult(t, firstResult); response.status != http.StatusBadRequest {
				t.Fatalf("failed import status = %d: %s", response.status, response.body)
			}
			second := waitDeploymentResult(t, secondResult)
			if second.status != http.StatusCreated {
				t.Fatalf("successful adopter status = %d: %s", second.status, second.body)
			}
			if !repository.rolledBack.Load() || repository.adoptedBeforeRollback.Load() {
				t.Fatal("another writer adopted the temporary asset before rollback completed")
			}
			var adoptedID string
			switch secondPath {
			case "/api/assets":
				var response struct {
					Asset game.Asset `json:"asset"`
				}
				if err := json.Unmarshal(second.body, &response); err != nil {
					t.Fatal(err)
				}
				adoptedID = response.Asset.ID
			case "/api/sounds":
				var response struct {
					Sound sound.Sound `json:"sound"`
				}
				if err := json.Unmarshal(second.body, &response); err != nil {
					t.Fatal(err)
				}
				adoptedID = response.Sound.AssetID
			default:
				var response struct {
					QuestionSet game.QuestionSet `json:"question_set"`
				}
				if err := json.Unmarshal(second.body, &response); err != nil {
					t.Fatal(err)
				}
				_, revision, err := games.GetQuestionSet(context.Background(), response.QuestionSet.ID)
				if err != nil {
					t.Fatal(err)
				}
				ids, err := referencedAssetIDs(revision.Data)
				if err != nil || len(ids) != 1 {
					t.Fatalf("committed reference missing: %v, %v", ids, err)
				}
				adoptedID = ids[0]
			}
			if adoptedID == firstAsset.ID {
				t.Fatal("successful writer reused the failed import's rolled-back asset")
			}
			asset, err := games.GetAsset(context.Background(), adoptedID)
			if err != nil {
				t.Fatalf("committed asset row missing: %v", err)
			}
			contents, err := os.ReadFile(filepath.Join(dataDir, "assets", asset.SHA256))
			if err != nil || !bytes.Equal(contents, assetData) {
				t.Fatalf("committed asset bytes missing/changed: %v", err)
			}
		})
	}
}

func TestAssetWriteGateCoversAdoptionPathsButNotReads(t *testing.T) {
	for _, requestPath := range []string{"/api/assets", "/API/Assets/", "/api/question-sets", "/api/question-sets/import", "/api/question-sets/id", "/api/sounds"} {
		if !isAssetWrite(http.MethodPost, requestPath) {
			t.Errorf("write path %q is not serialized", requestPath)
		}
		if isAssetWrite(http.MethodGet, requestPath) {
			t.Errorf("read path %q unnecessarily serialized", requestPath)
		}
	}
	if isAssetWrite(http.MethodPut, "/api/settings") {
		t.Fatal("unrelated settings mutation is unnecessarily serialized")
	}
}
