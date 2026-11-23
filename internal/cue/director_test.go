package cue

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/sound"
)

type fakePlayer struct {
	mu    sync.Mutex
	calls []string
	music string
	block chan struct{}
}

func (p *fakePlayer) record(call string) {
	p.mu.Lock()
	p.calls = append(p.calls, call)
	p.mu.Unlock()
}

func (p *fakePlayer) PlayAt(_ context.Context, id string, startAt int64) (sound.Playback, error) {
	if p.block != nil {
		<-p.block
	}
	p.record(fmt.Sprintf("play %s @%d", id, startAt))
	return sound.Playback{SoundID: id}, nil
}

func (p *fakePlayer) StopBus(request sound.StopBusRequest) error {
	p.record(fmt.Sprintf("stop %s", request.Bus))
	return nil
}

func (p *fakePlayer) StopMusic(id string, fadeMS int) bool {
	p.record(fmt.Sprintf("fade %s %d", id, fadeMS))
	return true
}

func (p *fakePlayer) MusicSoundID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.music
}

func (p *fakePlayer) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.calls...)
}

type fakeTimer struct {
	delay   time.Duration
	fn      func()
	stopped bool
}

func (t *fakeTimer) Stop() bool { t.stopped = true; return true }

type fakeClock struct {
	mu     sync.Mutex
	timers []*fakeTimer
}

func (c *fakeClock) afterFunc(delay time.Duration, fn func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	timer := &fakeTimer{delay: delay, fn: fn}
	c.timers = append(c.timers, timer)
	return timer
}

func (c *fakeClock) all() []*fakeTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*fakeTimer(nil), c.timers...)
}

func harness(t *testing.T, settings Settings) (*Director, *fakePlayer, *fakeClock) {
	t.Helper()
	player, clock := &fakePlayer{}, &fakeClock{}
	director := newDirector(player, func() Settings { return settings }, clock.afterFunc)
	t.Cleanup(director.Close)
	return director, player, clock
}

func waitProcessed(t *testing.T, d *Director, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		done := d.processed >= n
		d.mu.Unlock()
		if done {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("director did not process %d transitions", n)
}

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("calls\n got %q\nwant %q", got, want)
	}
}

func TestDirectorDelayedCueFires(t *testing.T) {
	d, player, clock := harness(t, enabled)
	d.Observe(state(controller.PhaseKhoiDong, controller.ScreenMainKD), state(controller.PhaseKhoiDong, controller.ScreenQuesChung))
	waitProcessed(t, d, 1)
	equal(t, player.snapshot(), []string{"play " + KDStartTurn + " @0"})
	timers := clock.all()
	if len(timers) != 1 || timers[0].delay != 4500*time.Millisecond || d.Pending() != 1 {
		t.Fatalf("want one 4.5 s follow-up, got %d timers", len(timers))
	}
	timers[0].fn()
	equal(t, player.snapshot(), []string{"play " + KDStartTurn + " @0", "play " + KDPreMainTime + " @0"})
	if d.Pending() != 0 {
		t.Fatal("fired cue must leave the pending set")
	}
}

func TestDirectorCancelsDelayedCueOnScreenChange(t *testing.T) {
	d, player, clock := harness(t, enabled)
	main := state(controller.PhaseVeDich, controller.ScreenMainVD)
	pre := state(controller.PhaseVeDich, controller.ScreenPreQuesTS1)
	d.Observe(main, pre)
	// The package is confirmed on the same screen: its ticks share the screen tag.
	confirmed := with(pre, func(s *game.RuntimeState) {
		s.Controller.VeDichContestantsState[0].SelectedQuestionSet = []string{"20_1", "20_2", "20_3"}
	})
	d.Observe(pre, confirmed)
	waitProcessed(t, d, 2)
	if d.Pending() != 3 { // PackageShow +5 s, ChoiceChosen +0.5 s, +1 s
		t.Fatalf("pending = %d, want 3", d.Pending())
	}
	clock.all()[1].fn() // ChoiceChosen +500 ms fires while still on pre_ques
	d.Observe(confirmed, state(controller.PhaseVeDich, controller.ScreenQuesTS1))
	waitProcessed(t, d, 3)
	if d.Pending() != 0 {
		t.Fatalf("screen change must cancel follow-ups, pending = %d", d.Pending())
	}
	for _, timer := range clock.all() {
		timer.fn() // late callbacks are ignored
	}
	equal(t, player.snapshot(), []string{
		"play " + VDStartTurn + " @0",
		"play " + VDChoiceChosen + " @0",
		"play " + VDChoiceChosen + " @0",
		"play " + VDPackageChoose + " @0",
	})
	stopped := 0
	for _, timer := range clock.all() {
		if timer.stopped {
			stopped++
		}
	}
	if stopped != 2 {
		t.Fatalf("stopped timers = %d, want 2", stopped)
	}
}

func TestDirectorMainTimeOnlyWhenIdle(t *testing.T) {
	d, player, _ := harness(t, enabled)
	screen := state(controller.PhaseKhoiDong, controller.ScreenQuesTS1)
	shown := with(screen, showing("kd_rieng.0.0"))
	player.music = KDMainTime
	d.Observe(screen, shown)
	waitProcessed(t, d, 1)
	equal(t, player.snapshot(), nil)
	player.mu.Lock()
	player.music = ""
	player.mu.Unlock()
	d.Observe(shown, with(screen, showing("kd_rieng.0.1")))
	d.Observe(with(screen, showing("kd_rieng.0.1")), state(controller.PhaseKhoiDong, controller.ScreenEndTS))
	waitProcessed(t, d, 3)
	equal(t, player.snapshot(), []string{"play " + KDMainTime + " @0", "play " + KDFinishTurn + " @0", "fade " + KDMainTime + " 700"})
}

func TestDirectorTimerChannel(t *testing.T) {
	d, player, _ := harness(t, enabled)
	q := with(state(controller.PhaseTangToc, controller.ScreenQues), showing("tt.1"))
	start := with(q, running(20000, 1000, "question"))
	paused := with(q, func(s *game.RuntimeState) {
		s.Timer = game.TimerState{IsRunning: true, IsPaused: true, DurationMS: 20000, RemainingMS: 15000, ElapsedMS: 5000}
	})
	resumed := with(q, func(s *game.RuntimeState) {
		s.Timer = game.TimerState{IsRunning: true, DurationMS: 20000, RemainingMS: 15000, ElapsedMS: 5000, StartedAt: 9000}
	})
	d.Observe(q, start)
	d.Observe(start, paused)
	d.Observe(paused, resumed)
	d.Observe(resumed, q)
	waitProcessed(t, d, 4)
	equal(t, player.snapshot(), []string{"play " + TT20Seconds + " @1000", "stop timer", "play " + TT20Seconds + " @4000", "stop timer"})
}

func TestDirectorObserveDoesNotBlock(t *testing.T) {
	player, clock := &fakePlayer{block: make(chan struct{})}, &fakeClock{}
	d := newDirector(player, func() Settings { return enabled }, clock.afterFunc)
	prev := state(controller.PhaseKhoiDong, controller.ScreenStart)
	cur := state(controller.PhaseKhoiDong, controller.ScreenIntro)
	done := make(chan struct{})
	go func() {
		for range 100 {
			d.Observe(prev, cur)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Observe blocked on a slow player")
	}
	close(player.block)
	d.Close()
}

func TestDirectorSoundDisabledSkipsDelayedCue(t *testing.T) {
	settings := enabled
	var mu sync.Mutex
	player, clock := &fakePlayer{}, &fakeClock{}
	d := newDirector(player, func() Settings { mu.Lock(); defer mu.Unlock(); return settings }, clock.afterFunc)
	defer d.Close()
	d.Observe(state(controller.PhaseKhoiDong, controller.ScreenMainKD), state(controller.PhaseKhoiDong, controller.ScreenQuesChung))
	waitProcessed(t, d, 1)
	mu.Lock()
	settings.Enabled = false
	mu.Unlock()
	clock.all()[0].fn()
	equal(t, player.snapshot(), []string{"play " + KDStartTurn + " @0"})
}
