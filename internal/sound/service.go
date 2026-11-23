package sound

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
	"github.com/onlyin32bit/4t/internal/settings"
)

type Repository interface {
	EnsureBuiltinSounds(context.Context, []Sound) error
	ListSounds(context.Context, bool) ([]Sound, error)
	GetSound(context.Context, string) (Sound, error)
	CreateSound(context.Context, Sound, game.Actor) (Sound, error)
	UpdateSound(context.Context, string, Update, game.Actor) (Sound, error)
	ArchiveSound(context.Context, string, game.Actor) error
}

type Publisher func(room, event string, data any)

type Service struct {
	mu          sync.Mutex
	repository  Repository
	settings    *settings.Service
	publish     Publisher
	outputCount func() int
	music       *Playback
	// questionMedia is the last Phát media playback; a re-press replaces it.
	questionMedia *Playback
}

func NewService(ctx context.Context, repository Repository, siteSettings *settings.Service, publish Publisher, outputCount func() int) (*Service, error) {
	service := &Service{repository: repository, settings: siteSettings, publish: publish, outputCount: outputCount}
	if err := repository.EnsureBuiltinSounds(ctx, Builtins()); err != nil {
		return nil, fmt.Errorf("seed built-in sounds: %w", err)
	}
	return service, nil
}

func (s *Service) List(ctx context.Context, includeDisabled bool) ([]Sound, error) {
	values, err := s.repository.ListSounds(ctx, includeDisabled)
	if err != nil {
		return nil, err
	}
	for i := range values {
		values[i].URL = soundURL(values[i])
	}
	return values, nil
}

func (s *Service) Get(ctx context.Context, id string) (Sound, error) {
	value, err := s.repository.GetSound(ctx, id)
	if err != nil {
		return Sound{}, &opsocket.Error{Code: "NOT_FOUND", Message: "sound not found"}
	}
	value.URL = soundURL(value)
	return value, nil
}

func (s *Service) Exists(ctx context.Context, id string) bool {
	if id == "" {
		return false
	}
	value, err := s.repository.GetSound(ctx, id)
	return err == nil && value.Enabled
}

func (s *Service) Create(ctx context.Context, assetID string, update Update, actor game.Actor) (Sound, error) {
	if err := validateUpdate(update); err != nil {
		return Sound{}, err
	}
	value, err := s.repository.CreateSound(ctx, Sound{
		ID: uuid.NewString(), Key: "upload." + uuid.NewString(), AssetID: assetID,
		Name: update.Name, Category: update.Category, SortOrder: update.SortOrder,
		Bus: update.Bus, Volume: update.Volume, Loop: update.Loop,
		FadeInMS: update.FadeInMS, FadeOutMS: update.FadeOutMS, Enabled: update.Enabled,
	}, actor)
	if err != nil {
		return Sound{}, err
	}
	s.catalogChanged()
	value.URL = soundURL(value)
	return value, nil
}

func (s *Service) Update(ctx context.Context, id string, update Update, actor game.Actor) (Sound, error) {
	if err := validateUpdate(update); err != nil {
		return Sound{}, err
	}
	if id == s.settings.Get().Sound.BellSoundID && !update.Enabled {
		return Sound{}, &opsocket.Error{Code: "VALIDATION", Message: "select another bell sound before disabling this one"}
	}
	value, err := s.repository.UpdateSound(ctx, id, update, actor)
	if err != nil {
		return Sound{}, err
	}
	s.catalogChanged()
	value.URL = soundURL(value)
	return value, nil
}

func (s *Service) Archive(ctx context.Context, id string, actor game.Actor) error {
	value, err := s.repository.GetSound(ctx, id)
	if err != nil {
		return err
	}
	if value.Builtin {
		return &opsocket.Error{Code: "VALIDATION", Message: "built-in sounds can be disabled but not deleted"}
	}
	if id == s.settings.Get().Sound.BellSoundID {
		return &opsocket.Error{Code: "VALIDATION", Message: "select another bell sound before deleting this one"}
	}
	if err := s.repository.ArchiveSound(ctx, id, actor); err != nil {
		return err
	}
	s.catalogChanged()
	return nil
}

func (s *Service) Play(ctx context.Context, id string) (Playback, error) {
	return s.PlayAt(ctx, id, 0)
}

// PlayAt plays a catalog sound. A non-zero startAt (server ms) schedules the
// sound at that instant; a past value makes outputs start part-way through,
// which lets a resumed timer sound continue at its offset.
func (s *Service) PlayAt(ctx context.Context, id string, startAt int64) (Playback, error) {
	if !s.settings.Get().Sound.Enabled {
		return Playback{}, &opsocket.Error{Code: "SOUND_DISABLED", Message: "sound output is disabled"}
	}
	value, err := s.Get(ctx, id)
	if err != nil || !value.Enabled {
		return Playback{}, &opsocket.Error{Code: "NOT_FOUND", Message: "sound not found or disabled"}
	}
	playback := playbackFor(value)
	if startAt > 0 {
		playback.StartAt = startAt
	}
	s.mu.Lock()
	if playback.Bus == BusMusic {
		if s.music != nil {
			s.emit(OutputRoom, "Sound.Stopped", StopEvent{PlaybackID: s.music.PlaybackID, FadeMS: quickFade(s.settings.Get())})
		}
		s.music = &playback
	}
	s.mu.Unlock()
	s.emit(OutputRoom, "Sound.Played", playback)
	return playback, nil
}

// MusicSoundID is the sound currently looping on the music bus ("" if none).
func (s *Service) MusicSoundID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.music == nil {
		return ""
	}
	return s.music.SoundID
}

// StopMusic fades the background music only when it is soundID.
func (s *Service) StopMusic(soundID string, fadeMS int) bool {
	s.mu.Lock()
	if s.music == nil || s.music.SoundID != soundID {
		s.mu.Unlock()
		return false
	}
	playbackID := s.music.PlaybackID
	s.mu.Unlock()
	s.Stop(StopRequest{PlaybackID: playbackID, FadeMS: fadeMS})
	return true
}

// PlayQuestionMedia plays the current question's audio asset through the TV
// sound output (effects bus, legacy question-audio loudness). A re-press stops
// the previous question-media playback first.
func (s *Service) PlayQuestionMedia(question *game.QuestionSnapshot) (Playback, error) {
	if !s.settings.Get().Sound.Enabled {
		return Playback{}, &opsocket.Error{Code: "SOUND_DISABLED", Message: "sound output is disabled"}
	}
	if question == nil || question.Type != "audio" || question.AssetID == "" {
		return Playback{}, &opsocket.Error{Code: "VALIDATION", Message: "the current question has no audio media"}
	}
	playback := Playback{
		PlaybackID: uuid.NewString(), SoundID: "asset:" + question.AssetID, URL: "/api/assets/" + question.AssetID,
		Bus: BusEffects, Volume: VolumeQuestionAudio, StartAt: time.Now().Add(120 * time.Millisecond).UnixMilli(),
	}
	s.mu.Lock()
	previous := s.questionMedia
	s.questionMedia = &playback
	s.mu.Unlock()
	if previous != nil {
		s.emit(OutputRoom, "Sound.Stopped", StopEvent{PlaybackID: previous.PlaybackID, FadeMS: quickFade(s.settings.Get())})
	}
	s.emit(OutputRoom, "Sound.Played", playback)
	return playback, nil
}

func (s *Service) Test(ctx context.Context) (Playback, error) {
	id := s.settings.Get().Sound.BellSoundID
	value, err := s.Get(ctx, id)
	if err != nil || !value.Enabled {
		value, err = s.Get(ctx, "builtin.kd.bell")
	}
	if err != nil {
		return Playback{}, err
	}
	playback := playbackFor(value)
	playback.Loop = false
	return playback, nil
}

func (s *Service) PlayBell(ctx context.Context) {
	if !s.settings.Get().Sound.Enabled {
		return
	}
	playback, err := s.Test(ctx)
	if err == nil {
		s.emit(OutputRoom, "Sound.Played", playback)
	}
}

func (s *Service) Broadcast(playback Playback) {
	s.emit(OutputRoom, "Sound.Played", playback)
}

func (s *Service) Stop(request StopRequest) {
	request.FadeMS = normalizedFade(request.FadeMS, s.settings.Get())
	s.mu.Lock()
	if s.music != nil && (request.PlaybackID == "" || request.PlaybackID == s.music.PlaybackID) {
		s.music = nil
	}
	if s.questionMedia != nil && (request.PlaybackID == "" || request.PlaybackID == s.questionMedia.PlaybackID) {
		s.questionMedia = nil
	}
	s.mu.Unlock()
	s.emit(OutputRoom, "Sound.Stopped", StopEvent{PlaybackID: request.PlaybackID, FadeMS: request.FadeMS})
}

func (s *Service) StopBus(request StopBusRequest) error {
	if !ValidBus(request.Bus) {
		return &opsocket.Error{Code: "BAD_REQUEST", Message: "unknown sound bus"}
	}
	request.FadeMS = normalizedFade(request.FadeMS, s.settings.Get())
	s.mu.Lock()
	switch request.Bus {
	case BusMusic:
		s.music = nil
	case BusEffects:
		s.questionMedia = nil
	}
	s.mu.Unlock()
	s.emit(OutputRoom, "Sound.Stopped", StopEvent{Bus: request.Bus, FadeMS: request.FadeMS})
	return nil
}

func (s *Service) StopAll(fadeMS int) {
	fadeMS = normalizedFade(fadeMS, s.settings.Get())
	s.mu.Lock()
	s.music = nil
	s.questionMedia = nil
	s.mu.Unlock()
	s.emit(OutputRoom, "Sound.Stopped", StopEvent{All: true, FadeMS: fadeMS})
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	var music *Playback
	if s.music != nil {
		copy := *s.music
		music = &copy
	}
	count := 0
	if s.outputCount != nil {
		count = s.outputCount()
	}
	return Status{OutputCount: count, Music: music}
}

func (s *Service) SettingsChanged() {
	s.emit(OutputRoom, "Sound.SettingsUpdated", s.settings.Get().Sound)
	if !s.settings.Get().Sound.Enabled {
		s.StopAll(quickFade(s.settings.Get()))
	}
}

func (s *Service) catalogChanged() {
	s.emit(OutputRoom, "Sound.CatalogUpdated", struct{}{})
	s.emit("role:admin", "Sound.CatalogUpdated", struct{}{})
}

func (s *Service) emit(room, event string, data any) {
	if s.publish != nil {
		s.publish(room, event, data)
	}
}

func soundURL(value Sound) string {
	if value.BuiltinPath != "" {
		return "/sounds/" + strings.TrimPrefix(value.BuiltinPath, "/")
	}
	return "/api/assets/" + value.AssetID
}

func playbackFor(value Sound) Playback {
	return Playback{
		PlaybackID: uuid.NewString(), SoundID: value.ID, URL: soundURL(value), Bus: value.Bus,
		Volume: value.Volume, Loop: value.Loop, FadeInMS: value.FadeInMS,
		FadeOutMS: value.FadeOutMS, StartAt: time.Now().Add(120 * time.Millisecond).UnixMilli(),
	}
}

func validateUpdate(value Update) error {
	value.Name = strings.TrimSpace(value.Name)
	value.Category = strings.TrimSpace(value.Category)
	if value.Name == "" || value.Category == "" {
		return &opsocket.Error{Code: "VALIDATION", Message: "sound name and category are required"}
	}
	if !ValidBus(value.Bus) {
		return &opsocket.Error{Code: "VALIDATION", Message: "sound bus must be music, timer or effects"}
	}
	if value.Volume < 0 || value.Volume > 1 || value.FadeInMS < 0 || value.FadeInMS > 10000 || value.FadeOutMS < 0 || value.FadeOutMS > 10000 {
		return &opsocket.Error{Code: "VALIDATION", Message: "invalid sound volume or fade duration"}
	}
	return nil
}

func normalizedFade(value int, site settings.Values) int {
	if value <= 0 {
		return quickFade(site)
	}
	if value > 10000 {
		return 10000
	}
	return value
}

func quickFade(site settings.Values) int { return site.Sound.TransitionFadeMS }

func Builtins() []Sound {
	paths := []string{
		"bonus/Thinking.mp3", "kd/3Seconds.m4a", "kd/Bell.mp3", "kd/CorrectAnswer.mp3",
		"kd/FinishTurn.mp3", "kd/MainTime.mp3", "kd/PreMainTime.mp3", "kd/StartRound.mp3",
		"kd/StartTurn.mp3", "kd/WrongAnswer.mp3", "main/EndSound.mp3", "main/HoiHop.m4a",
		"main/PointSummary.mp3", "main/Space.mp3", "main/Start.mp3", "tt/10Seconds.mp3",
		"tt/20Seconds.mp3", "tt/30Seconds.mp3", "tt/40Seconds.mp3", "tt/AnswersShowing.ogg",
		"tt/Correct.mp3", "tt/QuestionShowing.mp3", "tt/StartRound.mp3", "vcnv/15Seconds.mp3",
		"vcnv/AnswersShowing.ogg", "vcnv/CorrectObstacle.mp3", "vcnv/CorrectRow.mp3",
		"vcnv/ObstacleGrant.mp3", "vcnv/PictureReveal.mp3", "vcnv/RowChoose.mp3",
		"vcnv/RowQuestShow.mp3", "vcnv/RowsShow.mp3", "vcnv/StartRound.mp3",
		"vcnv/WrongRow.mp3", "vd/15Seconds.mp3", "vd/20Seconds.mp3", "vd/5Seconds.mp3",
		"vd/ChoiceChosen.mp3", "vd/CorrectFinish.mp3", "vd/FinishRound.mp3", "vd/Grant.mp3",
		"vd/PackageChoose.mp3", "vd/PackageShow.mp3", "vd/Practice20Seconds.mp3",
		"vd/Practice30Seconds.mp3", "vd/Practice40Seconds.mp3", "vd/Practice60Seconds.mp3",
		"vd/StarChoose.mp3", "vd/StartRound.mp3", "vd/StartTurn.mp3", "vd/WrongFinish.mp3",
	}
	result := make([]Sound, 0, len(paths))
	for order, file := range paths {
		category, filename := path.Split(file)
		category = strings.TrimSuffix(category, "/")
		name := strings.TrimSuffix(filename, path.Ext(filename))
		keyName := strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return unicode.ToLower(r)
			}
			return '-'
		}, name)
		bus, volume, loop := builtinMix(file)
		id := "builtin." + category + "." + keyName
		result = append(result, Sound{ID: id, Key: id, Name: name, Category: category, SortOrder: order, Bus: bus, Volume: volume, Loop: loop, FadeOutMS: 120, BuiltinPath: file, Builtin: true, Enabled: true})
	}
	return result
}

// BuiltinTimerSounds are the built-in sounds on the single-slot timer bus.
var BuiltinTimerSounds = []string{
	"kd/3Seconds.m4a", "tt/10Seconds.mp3", "tt/20Seconds.mp3", "tt/30Seconds.mp3", "tt/40Seconds.mp3",
	"vcnv/15Seconds.mp3", "bonus/Thinking.mp3", "vd/5Seconds.mp3", "vd/15Seconds.mp3", "vd/20Seconds.mp3",
	"vd/Practice20Seconds.mp3", "vd/Practice30Seconds.mp3", "vd/Practice40Seconds.mp3", "vd/Practice60Seconds.mp3",
}

// BuiltinMusicSound is the only built-in background loop.
const BuiltinMusicSound = "kd/MainTime.mp3"

// builtinMix classifies a built-in sound (keep migration 008 in sync).
func builtinMix(file string) (Bus, float64, bool) {
	if file == BuiltinMusicSound {
		return BusMusic, VolumeMusic, true
	}
	for _, timer := range BuiltinTimerSounds {
		if file == timer {
			return BusTimer, VolumeTimer, false
		}
	}
	return BusEffects, VolumeEffects, false
}
