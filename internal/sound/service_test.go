package sound

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"sync"
	"testing"

	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/settings"
)

type memoryRepository struct{ sounds map[string]Sound }

func (r *memoryRepository) EnsureBuiltinSounds(_ context.Context, values []Sound) error {
	r.sounds = map[string]Sound{}
	for _, value := range values {
		r.sounds[value.ID] = value
	}
	return nil
}
func (r *memoryRepository) ListSounds(context.Context, bool) ([]Sound, error) { return nil, nil }
func (r *memoryRepository) GetSound(_ context.Context, id string) (Sound, error) {
	value, ok := r.sounds[id]
	if !ok {
		return Sound{}, errors.New("missing")
	}
	return value, nil
}
func (r *memoryRepository) CreateSound(context.Context, Sound, game.Actor) (Sound, error) {
	return Sound{}, nil
}
func (r *memoryRepository) UpdateSound(context.Context, string, Update, game.Actor) (Sound, error) {
	return Sound{}, nil
}
func (r *memoryRepository) ArchiveSound(context.Context, string, game.Actor) error { return nil }

type emitted struct {
	event string
	data  any
}

func newTestService(t *testing.T) (*Service, *[]emitted) {
	t.Helper()
	site, err := settings.Open(filepath.Join(t.TempDir(), "settings.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	events := &[]emitted{}
	service, err := NewService(context.Background(), &memoryRepository{}, site, func(_, event string, data any) {
		mu.Lock()
		*events = append(*events, emitted{event, data})
		mu.Unlock()
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return service, events
}

func TestBuiltinClassification(t *testing.T) {
	timer := map[string]bool{}
	for _, file := range BuiltinTimerSounds {
		timer[file] = true
	}
	for _, value := range Builtins() {
		switch {
		case value.BuiltinPath == "kd/MainTime.mp3":
			if value.Bus != BusMusic || !value.Loop || value.Volume != VolumeMusic {
				t.Fatalf("MainTime: %+v", value)
			}
		case timer[value.BuiltinPath]:
			if value.Bus != BusTimer || value.Loop || value.Volume != VolumeTimer {
				t.Fatalf("%s: %+v", value.BuiltinPath, value)
			}
		default:
			if value.Bus != BusEffects || value.Loop || value.Volume != VolumeEffects {
				t.Fatalf("%s: %+v", value.BuiltinPath, value)
			}
		}
	}
	// Legacy ratio effects .5 : timer .4 : music .4 : question audio .6.
	if math.Abs(VolumeEffects/VolumeQuestionAudio-0.5/0.6) > 1e-9 || math.Abs(VolumeTimer/VolumeEffects-0.8) > 1e-9 || VolumeMusic != VolumeTimer {
		t.Fatal("volume ratio drifted from legacy")
	}
}

func TestPlayAtAndTimerBus(t *testing.T) {
	service, events := newTestService(t)
	playback, err := service.PlayAt(context.Background(), "builtin.tt.30seconds", 12345)
	if err != nil {
		t.Fatal(err)
	}
	if playback.Bus != BusTimer || playback.StartAt != 12345 {
		t.Fatalf("timer playback: %+v", playback)
	}
	if err := service.StopBus(StopBusRequest{Bus: BusTimer}); err != nil {
		t.Fatal(err)
	}
	last := (*events)[len(*events)-1]
	if stop, ok := last.data.(StopEvent); last.event != "Sound.Stopped" || !ok || stop.Bus != BusTimer {
		t.Fatalf("stop timer bus: %+v", last)
	}
	if err := service.StopBus(StopBusRequest{Bus: "bogus"}); err == nil {
		t.Fatal("unknown bus must fail")
	}
	if err := validateUpdate(Update{Name: "x", Category: "y", Bus: BusTimer, Volume: 1}); err != nil {
		t.Fatalf("timer bus update: %v", err)
	}
}

func TestStopMusicOnlyStopsThatSound(t *testing.T) {
	service, _ := newTestService(t)
	if _, err := service.Play(context.Background(), "builtin.kd.maintime"); err != nil {
		t.Fatal(err)
	}
	if service.MusicSoundID() != "builtin.kd.maintime" {
		t.Fatal("MainTime should be the music loop")
	}
	if service.StopMusic("builtin.other", 700) || service.MusicSoundID() == "" {
		t.Fatal("other sound must not stop the loop")
	}
	if !service.StopMusic("builtin.kd.maintime", 700) || service.MusicSoundID() != "" {
		t.Fatal("MainTime should fade")
	}
}

func TestPlayQuestionMediaRestarts(t *testing.T) {
	service, events := newTestService(t)
	if _, err := service.PlayQuestionMedia(&game.QuestionSnapshot{ID: "q", Type: "image", AssetID: "a"}); err == nil {
		t.Fatal("non-audio question must be rejected")
	}
	if _, err := service.PlayQuestionMedia(nil); err == nil {
		t.Fatal("no question must be rejected")
	}
	question := &game.QuestionSnapshot{ID: "q", Type: "audio", AssetID: "asset-1"}
	first, err := service.PlayQuestionMedia(question)
	if err != nil {
		t.Fatal(err)
	}
	if first.Bus != BusEffects || first.Volume != VolumeQuestionAudio || first.URL != "/api/assets/asset-1" {
		t.Fatalf("question media playback: %+v", first)
	}
	*events = nil
	second, err := service.PlayQuestionMedia(question)
	if err != nil {
		t.Fatal(err)
	}
	if len(*events) != 2 || (*events)[0].event != "Sound.Stopped" || (*events)[1].event != "Sound.Played" {
		t.Fatalf("re-press must stop then play: %+v", *events)
	}
	if stop := (*events)[0].data.(StopEvent); stop.PlaybackID != first.PlaybackID {
		t.Fatalf("stopped %s, want %s", stop.PlaybackID, first.PlaybackID)
	}
	if second.PlaybackID == first.PlaybackID {
		t.Fatal("re-press needs a new playback")
	}
}
