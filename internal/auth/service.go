package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUsernameRequired   = errors.New("username is required")
	ErrPasswordTooShort   = errors.New("password must contain at least 8 characters")
)

type Store interface {
	HasAdmin(context.Context) (bool, error)
	CreateUser(context.Context, string, string, Role) (User, error)
	GetUserByID(context.Context, int64) (User, error)
	GetUserByUsername(context.Context, string) (User, error)
	CreateSession(context.Context, int64, string, string, string, time.Time) error
	ReplaceSession(context.Context, int64, string, string, string, time.Time) error
	GetSession(context.Context, string) (int64, time.Time, error)
	UpdateSessionExpiration(context.Context, string, time.Time) error
	TouchSession(context.Context, string, time.Time) error
	DeleteSession(context.Context, string) error
	ListSessions(context.Context) ([]Session, error)
	CreateFixedContestant(context.Context, int, string, string) (User, error)
	ListFixedContestants(context.Context) ([]User, error)
	UpdateFixedContestantPassword(context.Context, int, string) error
	DeleteSessionsByUser(context.Context, int64) error
	ListAdmins(context.Context) ([]User, error)
	UpdateUserPassword(context.Context, int64, string) error
	UpdateUserPasswordAndDeleteSessions(context.Context, int64, string) error
	SetUserDisabled(context.Context, int64, bool) error
	CountEnabledAdmins(context.Context) (int, error)
}

type Service struct {
	store     Store
	mu        sync.Mutex
	failures  map[string]loginFailure
	dummyHash []byte
}

type loginFailure struct {
	Count        int
	BlockedUntil time.Time
	LastFailed   time.Time
}

func NewService(store Store) *Service {
	dummyHash, _ := bcrypt.GenerateFromPassword([]byte("4t-invalid-login-probe"), bcrypt.DefaultCost)
	return &Service{store: store, failures: make(map[string]loginFailure), dummyHash: dummyHash}
}

func (s *Service) HasAdmin(ctx context.Context) (bool, error) {
	return s.store.HasAdmin(ctx)
}

func (s *Service) CreateAdmin(ctx context.Context, username, password string) (UserDTO, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return UserDTO{}, ErrUsernameRequired
	}
	if len(password) < 8 {
		return UserDTO{}, ErrPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return UserDTO{}, fmt.Errorf("hash password: %w", err)
	}
	u, err := s.store.CreateUser(ctx, username, string(hash), RoleAdmin)
	if err != nil {
		return UserDTO{}, err
	}
	return u.DTO(), nil
}

func (s *Service) Login(ctx context.Context, username, password string) (string, time.Time, error) {
	return s.LoginFrom(ctx, username, password, "local")
}

// LoginFrom scopes throttling to both the account and the network source. This
// prevents one client from brute-forcing the six-digit event accounts without
// letting that client lock the account for every other contestant device.
func (s *Service) LoginFrom(ctx context.Context, username, password, source string, userAgents ...string) (string, time.Time, error) {
	normalizedUsername := strings.ToLower(strings.TrimSpace(username))
	key := strings.TrimSpace(source) + "|" + normalizedUsername
	s.mu.Lock()
	now := time.Now()
	s.pruneFailuresLocked(now)
	failure := s.failures[key]
	if now.Before(failure.BlockedUntil) {
		s.mu.Unlock()
		return "", time.Time{}, ErrInvalidCredentials
	}
	s.mu.Unlock()
	u, lookupErr := s.store.GetUserByUsername(ctx, strings.TrimSpace(username))
	hash := s.dummyHash
	if lookupErr == nil {
		hash = []byte(u.PasswordHash)
	}
	passwordErr := bcrypt.CompareHashAndPassword(hash, []byte(password))
	if lookupErr != nil || passwordErr != nil || u.Disabled {
		s.mu.Lock()
		now := time.Now()
		failure := s.failures[key]
		failure.Count++
		failure.LastFailed = now
		if failure.Count >= 5 {
			delay := time.Duration(1<<min(failure.Count-5, 6)) * time.Minute
			failure.BlockedUntil = now.Add(delay)
		}
		s.failures[key] = failure
		s.pruneFailuresLocked(now)
		s.mu.Unlock()
		return "", time.Time{}, ErrInvalidCredentials
	}
	s.mu.Lock()
	delete(s.failures, key)
	s.mu.Unlock()

	token, sessionID, err := generateSessionToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(15 * 24 * time.Hour)
	userAgent := ""
	if len(userAgents) > 0 {
		userAgent = strings.TrimSpace(userAgents[0])
	}
	if len(source) > 128 {
		source = source[:128]
	}
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	create := s.store.CreateSession
	if u.Role == RoleContestant {
		create = s.store.ReplaceSession
	}
	if err := create(ctx, u.ID, sessionID, source, userAgent, expiresAt); err != nil {
		return "", time.Time{}, fmt.Errorf("create session: %w", err)
	}
	return token, expiresAt, nil
}

func (s *Service) pruneFailuresLocked(now time.Time) {
	const maxEntries = 4096
	cutoff := now.Add(-time.Hour)
	for key, failure := range s.failures {
		if failure.LastFailed.IsZero() || failure.LastFailed.Before(cutoff) {
			delete(s.failures, key)
		}
	}
	for len(s.failures) > maxEntries {
		var oldestKey string
		var oldest time.Time
		for key, failure := range s.failures {
			if oldestKey == "" || failure.LastFailed.Before(oldest) {
				oldestKey, oldest = key, failure.LastFailed
			}
		}
		delete(s.failures, oldestKey)
	}
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.store.DeleteSession(ctx, sessionID(token))
}

func (s *Service) SessionID(token string) string { return sessionID(token) }

func (s *Service) ValidateSession(ctx context.Context, token string) (UserDTO, bool) {
	id := sessionID(token)
	userID, expiresAt, err := s.store.GetSession(ctx, id)
	if err != nil {
		return UserDTO{}, false
	}
	if time.Now().Compare(expiresAt) >= 0 {
		_ = s.store.DeleteSession(ctx, id)
		return UserDTO{}, false
	}

	// Keep the existing renewal semantics during this structural refactor.
	if time.Now().After(expiresAt.Add(-7 * 24 * time.Hour)) {
		_ = s.store.UpdateSessionExpiration(ctx, id, time.Now().Add(30*24*time.Hour))
	}

	u, err := s.store.GetUserByID(ctx, userID)
	if err != nil || u.Disabled {
		_ = s.store.DeleteSession(ctx, id)
		return UserDTO{}, false
	}
	_ = s.store.TouchSession(ctx, id, time.Now())
	return u.DTO(), true
}

func (s *Service) ListSessions(ctx context.Context, currentToken string) ([]SessionDTO, error) {
	values, err := s.store.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	currentID := sessionID(currentToken)
	now := time.Now()
	result := make([]SessionDTO, len(values))
	for i, value := range values {
		result[i] = SessionDTO{
			ID: value.ID, UserID: value.UserID, Username: value.Username, Role: value.Role,
			FixedSlot: value.FixedSlot, IPAddress: value.IPAddress, UserAgent: value.UserAgent,
			CreatedAt: value.CreatedAt.UTC().Format(time.RFC3339), LastSeen: value.LastSeen.UTC().Format(time.RFC3339),
			ExpiresAt: value.ExpiresAt.UTC().Format(time.RFC3339), Online: now.Sub(value.LastSeen) < 75*time.Second,
			Current: value.ID == currentID,
		}
	}
	return result, nil
}

func (s *Service) InvalidateSession(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("session id is required")
	}
	return s.store.DeleteSession(ctx, id)
}

func (s *Service) ListAdmins(ctx context.Context) ([]UserDTO, error) {
	users, err := s.store.ListAdmins(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]UserDTO, len(users))
	for i := range users {
		result[i] = users[i].DTO()
	}
	return result, nil
}

func (s *Service) ResetAdminPassword(ctx context.Context, id int64, password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	user, err := s.store.GetUserByID(ctx, id)
	if err != nil || user.Role != RoleAdmin {
		return errors.New("administrator account not found")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if err := s.store.UpdateUserPasswordAndDeleteSessions(ctx, id, string(hash)); err != nil {
		return err
	}
	return nil
}

func (s *Service) SetAdminEnabled(ctx context.Context, id, actorID int64, enabled bool) error {
	user, err := s.store.GetUserByID(ctx, id)
	if err != nil || user.Role != RoleAdmin {
		return errors.New("administrator account not found")
	}
	if !enabled {
		if id == actorID {
			return errors.New("you cannot disable your own account")
		}
		count, err := s.store.CountEnabledAdmins(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return errors.New("at least one administrator must remain enabled")
		}
	}
	if err := s.store.SetUserDisabled(ctx, id, !enabled); err != nil {
		return err
	}
	if !enabled {
		return s.store.DeleteSessionsByUser(ctx, id)
	}
	return nil
}

type InitialCredential struct {
	Username string
	Password string
}

func (s *Service) EnsureFixedContestants(ctx context.Context) ([]InitialCredential, error) {
	existing, err := s.store.ListFixedContestants(ctx)
	if err != nil {
		return nil, err
	}
	bySlot := make(map[int]User, len(existing))
	for _, user := range existing {
		bySlot[user.FixedSlot] = user
	}
	created := make([]InitialCredential, 0, 4-len(existing))
	for slot := 1; slot <= 4; slot++ {
		if _, ok := bySlot[slot]; ok {
			continue
		}
		password, err := randomSixDigits()
		if err != nil {
			return nil, err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash contestant password: %w", err)
		}
		username := fmt.Sprintf("thi_sinh_%d", slot)
		if _, err := s.store.CreateFixedContestant(ctx, slot, username, string(hash)); err != nil {
			return nil, fmt.Errorf("create %s: %w", username, err)
		}
		created = append(created, InitialCredential{Username: username, Password: password})
	}
	return created, nil
}

func (s *Service) ListFixedContestants(ctx context.Context) ([]UserDTO, error) {
	users, err := s.store.ListFixedContestants(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]UserDTO, len(users))
	for i := range users {
		result[i] = users[i].DTO()
	}
	return result, nil
}

func (s *Service) ResetFixedContestantPassword(ctx context.Context, slot int, password string) error {
	if slot < 1 || slot > 4 {
		return errors.New("contestant slot must be between 1 and 4")
	}
	if len(password) != 6 {
		return errors.New("contestant password must contain exactly 6 digits")
	}
	if _, err := strconv.Atoi(password); err != nil {
		return errors.New("contestant password must contain only digits")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash contestant password: %w", err)
	}
	users, err := s.store.ListFixedContestants(ctx)
	if err != nil {
		return err
	}
	var userID int64
	for _, user := range users {
		if user.FixedSlot == slot {
			userID = user.ID
			break
		}
	}
	if userID == 0 {
		return errors.New("contestant account not found")
	}
	if err := s.store.UpdateFixedContestantPassword(ctx, slot, string(hash)); err != nil {
		return err
	}
	if err := s.store.DeleteSessionsByUser(ctx, userID); err != nil {
		return err
	}
	s.mu.Lock()
	suffix := "|" + strings.ToLower(fmt.Sprintf("thi_sinh_%d", slot))
	for key := range s.failures {
		if strings.HasSuffix(key, suffix) {
			delete(s.failures, key)
		}
	}
	s.mu.Unlock()
	return nil
}

func randomSixDigits() (string, error) {
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate contestant password: %w", err)
	}
	value := (uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3])) % 1_000_000
	return fmt.Sprintf("%06d", value), nil
}

func generateSessionToken() (string, string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, sessionID(token), nil
}

func sessionID(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
