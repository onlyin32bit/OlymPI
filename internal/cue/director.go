package cue

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/sound"
)

// Player executes cues on the sound outputs (implemented by *sound.Service).
type Player interface {
	PlayAt(ctx context.Context, id string, startAt int64) (sound.Playback, error)
	StopBus(request sound.StopBusRequest) error
	StopMusic(soundID string, fadeMS int) bool
	MusicSoundID() string
}

// Timer is a cancellable scheduled callback (time.Timer in production).
type Timer interface{ Stop() bool }

// tag identifies the screen a delayed cue belongs to; it is dropped as soon
// as the director observes a different match, epoch, phase or screen.
type tag struct {
	matchID string
	epoch   int64
	phase   controller.Phase
	screen  controller.Screen
}

func tagOf(state game.RuntimeState) tag {
	return tag{state.MatchID, state.Epoch, state.Controller.Phase, state.Controller.Screen}
}

type transition struct{ prev, cur game.RuntimeState }

type pending struct {
	tag   tag
	cue   Cue
	timer Timer
}

// Director turns game state changes into sound cues. Observe never blocks:
// transitions are queued and processed in order on the director's goroutine.
type Director struct {
	player    Player
	settings  func() Settings
	afterFunc func(time.Duration, func()) Timer

	mu      sync.Mutex
	queue   []transition
	wake    chan struct{}
	current tag
	pending map[*pending]struct{}
	closed  bool
	done    chan struct{}
	// processed counts handled transitions (tests wait on it).
	processed int
}

// New starts a director. settings is read for every transition.
func New(player Player, settings func() Settings) *Director {
	return newDirector(player, settings, func(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) })
}

func newDirector(player Player, settings func() Settings, afterFunc func(time.Duration, func()) Timer) *Director {
	d := &Director{
		player: player, settings: settings, afterFunc: afterFunc,
		wake: make(chan struct{}, 1), pending: map[*pending]struct{}{}, done: make(chan struct{}),
	}
	go d.run()
	return d
}

// Observe is a game.Service observer: it only enqueues and returns.
func (d *Director) Observe(prev, cur game.RuntimeState) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.queue = append(d.queue, transition{prev, cur})
	select {
	case d.wake <- struct{}{}:
	default:
	}
	d.mu.Unlock()
}

// Close stops the director and cancels delayed cues.
func (d *Director) Close() {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	for item := range d.pending {
		item.timer.Stop()
		delete(d.pending, item)
	}
	close(d.wake)
	d.mu.Unlock()
	<-d.done
}

func (d *Director) run() {
	defer close(d.done)
	for range d.wake {
		for {
			d.mu.Lock()
			if len(d.queue) == 0 || d.closed {
				d.mu.Unlock()
				break
			}
			next := d.queue[0]
			d.queue[0] = transition{}
			d.queue = d.queue[1:]
			d.mu.Unlock()
			d.process(next.prev, next.cur)
			d.mu.Lock()
			d.processed++
			d.mu.Unlock()
		}
	}
}

func (d *Director) process(prev, cur game.RuntimeState) {
	settings := d.settings()
	current := tagOf(cur)
	d.mu.Lock()
	d.current = current
	for item := range d.pending {
		if item.tag != current {
			item.timer.Stop()
			delete(d.pending, item)
		}
	}
	d.mu.Unlock()
	for _, value := range Cues(prev, cur, settings) {
		if value.DelayMS <= 0 {
			d.execute(value)
			continue
		}
		d.schedule(current, value)
	}
}

func (d *Director) schedule(at tag, value Cue) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	item := &pending{tag: at, cue: value}
	item.timer = d.afterFunc(time.Duration(value.DelayMS)*time.Millisecond, func() { d.fire(item) })
	d.pending[item] = struct{}{}
}

func (d *Director) fire(item *pending) {
	d.mu.Lock()
	_, live := d.pending[item]
	delete(d.pending, item)
	live = live && !d.closed && d.current == item.tag
	d.mu.Unlock()
	if !live {
		return
	}
	value := item.cue
	value.DelayMS = 0
	if d.settings().Enabled {
		d.execute(value)
	}
}

// Pending is the number of scheduled, not yet fired delayed cues.
func (d *Director) Pending() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.pending)
}

func (d *Director) execute(value Cue) {
	switch value.Op {
	case OpPlay:
		if _, err := d.player.PlayAt(context.Background(), value.SoundID, value.StartAt); err != nil {
			slog.Debug("sound cue skipped", "sound", value.SoundID, "error", err)
		}
	case OpPlayIfIdle:
		if d.player.MusicSoundID() == value.SoundID {
			return
		}
		if _, err := d.player.PlayAt(context.Background(), value.SoundID, value.StartAt); err != nil {
			slog.Debug("sound cue skipped", "sound", value.SoundID, "error", err)
		}
	case OpStopBus:
		_ = d.player.StopBus(sound.StopBusRequest{Bus: value.Bus, FadeMS: value.FadeMS})
	case OpFadeMusic:
		d.player.StopMusic(value.SoundID, value.FadeMS)
	}
}
