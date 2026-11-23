// Package cue is the server-side sound cue director: it derives the legacy
// audience sounds from consecutive runtime states (spec §5) and plays them on
// the TV sound outputs.
package cue

import (
	"slices"

	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/sound"
)

// Built-in sound IDs (sound.Builtins: "builtin.<category>.<lowercased name>").
const (
	MainStart          = "builtin.main.start"
	KDStartRound       = "builtin.kd.startround"
	KDStartTurn        = "builtin.kd.startturn"
	KDPreMainTime      = "builtin.kd.premaintime"
	KDMainTime         = "builtin.kd.maintime"
	KD3Seconds         = "builtin.kd.3seconds"
	KDFinishTurn       = "builtin.kd.finishturn"
	KDBell             = "builtin.kd.bell"
	VCNVStartRound     = "builtin.vcnv.startround"
	VCNVRowsShow       = "builtin.vcnv.rowsshow"
	VCNVRowChoose      = "builtin.vcnv.rowchoose"
	VCNVRowQuestShow   = "builtin.vcnv.rowquestshow"
	VCNV15Seconds      = "builtin.vcnv.15seconds"
	VCNVPictureReveal  = "builtin.vcnv.picturereveal"
	VCNVObstacleGrant  = "builtin.vcnv.obstaclegrant"
	VCNVAnswersShowing = "builtin.vcnv.answersshowing"
	BonusThinking      = "builtin.bonus.thinking"
	TTStartRound       = "builtin.tt.startround"
	TTQuestionShowing  = "builtin.tt.questionshowing"
	TTAnswersShowing   = "builtin.tt.answersshowing"
	TT10Seconds        = "builtin.tt.10seconds"
	TT20Seconds        = "builtin.tt.20seconds"
	TT30Seconds        = "builtin.tt.30seconds"
	TT40Seconds        = "builtin.tt.40seconds"
	VDStartRound       = "builtin.vd.startround"
	VDStartTurn        = "builtin.vd.startturn"
	VDPackageShow      = "builtin.vd.packageshow"
	VDChoiceChosen     = "builtin.vd.choicechosen"
	VDPackageChoose    = "builtin.vd.packagechoose"
	VDStarChoose       = "builtin.vd.starchoose"
	VD5Seconds         = "builtin.vd.5seconds"
	VD15Seconds        = "builtin.vd.15seconds"
	VD20Seconds        = "builtin.vd.20seconds"
	VDGrant            = "builtin.vd.grant"
	VDFinishRound      = "builtin.vd.finishround"
)

const (
	// KDPreMainTimeDelayMS follows kd/StartTurn when a KĐ turn opens.
	KDPreMainTimeDelayMS = 4500
	// VDPackageShowDelayMS follows vd/StartTurn when a VĐ turn opens.
	VDPackageShowDelayMS = 5000
	// VDChoiceIntervalMS separates the three vd/ChoiceChosen ticks.
	VDChoiceIntervalMS = 500
	// MainTimeFadeMS fades the KĐ background loop.
	MainTimeFadeMS = 700
)

type Op string

const (
	// OpPlay plays SoundID (timer cues carry StartAt so a resume continues at its offset).
	OpPlay Op = "play"
	// OpPlayIfIdle plays SoundID unless it is already the playing music loop.
	OpPlayIfIdle Op = "play_if_idle"
	// OpStopBus stops every playback on Bus.
	OpStopBus Op = "stop_bus"
	// OpFadeMusic fades SoundID out if it is the playing music loop.
	OpFadeMusic Op = "fade_music"
)

// Cue is one sound action. Bus documents the built-in bus of a played sound.
type Cue struct {
	Op      Op
	SoundID string
	Bus     sound.Bus
	DelayMS int64
	FadeMS  int
	// StartAt is the server time (ms) at which the sound's offset 0 lies; 0 = now.
	StartAt int64
}

// Settings are the site settings the director needs.
type Settings struct {
	Enabled     bool
	BellSoundID string
}

func play(id string, bus sound.Bus) Cue { return Cue{Op: OpPlay, SoundID: id, Bus: bus} }
func effect(id string) Cue              { return play(id, sound.BusEffects) }
func delayed(id string, ms int64) Cue {
	value := effect(id)
	value.DelayMS = ms
	return value
}

// Cues returns the sounds caused by the change from prev to cur. It is pure.
func Cues(prev, cur game.RuntimeState, settings Settings) []Cue {
	if prev.MatchID == "" || cur.MatchID == "" || prev.MatchID != cur.MatchID || prev.Epoch != cur.Epoch {
		return nil
	}
	if !settings.Enabled {
		return nil
	}
	var cues []Cue
	cues = append(cues, screenCues(prev, cur)...)
	cues = append(cues, mainTimeCues(prev, cur)...)
	cues = append(cues, timerCues(prev, cur)...)
	cues = append(cues, bellCues(prev, cur, settings)...)
	cues = append(cues, vcnvCues(prev, cur)...)
	cues = append(cues, veDichCues(prev, cur)...)
	return cues
}

func screenChanged(prev, cur game.RuntimeState) bool {
	return prev.Controller.Phase != cur.Controller.Phase || prev.Controller.Screen != cur.Controller.Screen
}

func isKDQuestionScreen(phase controller.Phase, screen controller.Screen) bool {
	if phase != controller.PhaseKhoiDong {
		return false
	}
	switch screen {
	case controller.ScreenQuesChung, controller.ScreenQuesTS1, controller.ScreenQuesTS2, controller.ScreenQuesTS3, controller.ScreenQuesTS4:
		return true
	}
	return false
}

func isVDPreQuestion(phase controller.Phase, screen controller.Screen) bool {
	if phase != controller.PhaseVeDich {
		return false
	}
	switch screen {
	case controller.ScreenPreQuesTS1, controller.ScreenPreQuesTS2, controller.ScreenPreQuesTS3, controller.ScreenPreQuesTS4:
		return true
	}
	return false
}

func isVDQuestion(phase controller.Phase, screen controller.Screen) bool {
	if phase != controller.PhaseVeDich {
		return false
	}
	switch screen {
	case controller.ScreenQuesTS1, controller.ScreenQuesTS2, controller.ScreenQuesTS3, controller.ScreenQuesTS4:
		return true
	}
	return false
}

func screenCues(prev, cur game.RuntimeState) []Cue {
	if !screenChanged(prev, cur) {
		return nil
	}
	phase, screen := cur.Controller.Phase, cur.Controller.Screen
	prevPhase, prevScreen := prev.Controller.Phase, prev.Controller.Screen
	if !controller.IsRound(phase) {
		// Idle, Logo, Main and Score are silent.
		return nil
	}
	switch screen {
	case controller.ScreenStart:
		if phase == controller.PhaseKhoiDong {
			return []Cue{effect(KDStartRound)}
		}
		return []Cue{effect(MainStart)}
	case controller.ScreenIntro:
		switch phase {
		case controller.PhaseKhoiDong:
			return []Cue{effect(KDStartTurn)}
		case controller.PhaseVCNV:
			return []Cue{effect(VCNVStartRound)}
		case controller.PhaseTangToc:
			return []Cue{effect(TTStartRound)}
		case controller.PhaseVeDich:
			return []Cue{effect(VDStartRound)}
		}
	case controller.ScreenEndTS:
		if phase == controller.PhaseKhoiDong || phase == controller.PhaseVeDich {
			return []Cue{effect(KDFinishTurn)}
		}
	case controller.ScreenEnd:
		return []Cue{effect(VDFinishRound)}
	case controller.ScreenAnswersVCNV:
		if phase == controller.PhaseVCNV {
			return []Cue{effect(VCNVAnswersShowing)}
		}
	case controller.ScreenAnswersTT:
		if phase == controller.PhaseTangToc {
			return []Cue{effect(TTAnswersShowing)}
		}
	case controller.ScreenQues:
		switch phase {
		case controller.PhaseVCNV:
			return []Cue{effect(VCNVRowQuestShow)}
		case controller.PhaseTangToc:
			return []Cue{effect(TTQuestionShowing)}
		}
	}
	if isKDQuestionScreen(phase, screen) && !isKDQuestionScreen(prevPhase, prevScreen) {
		return []Cue{effect(KDStartTurn), delayed(KDPreMainTime, KDPreMainTimeDelayMS)}
	}
	if isVDPreQuestion(phase, screen) && !isVDPreQuestion(prevPhase, prevScreen) {
		return []Cue{effect(VDStartTurn), delayed(VDPackageShow, VDPackageShowDelayMS)}
	}
	if isVDQuestion(phase, screen) && !isVDQuestion(prevPhase, prevScreen) {
		return []Cue{effect(VDPackageChoose)}
	}
	// Rule, main_kd, test_bell, main_vcnv, image_vcnv, TT solve, main_vd: silent.
	return nil
}

// kdQuestionShown: a KĐ question is revealed on a KĐ question screen.
func kdQuestionShown(state game.RuntimeState) bool {
	return isKDQuestionScreen(state.Controller.Phase, state.Controller.Screen) &&
		state.Controller.IsShowingQuestion && state.Controller.SelectedQuestionID != ""
}

// kdFinished is the KĐ "KẾT THÚC" end card.
func kdFinished(state game.RuntimeState) bool {
	return isKDQuestionScreen(state.Controller.Phase, state.Controller.Screen) &&
		state.Controller.IsShowingQuestion && state.Controller.SelectedQuestionID == ""
}

func mainTimeCues(prev, cur game.RuntimeState) []Cue {
	prevOn := isKDQuestionScreen(prev.Controller.Phase, prev.Controller.Screen)
	curOn := isKDQuestionScreen(cur.Controller.Phase, cur.Controller.Screen)
	fade := Cue{Op: OpFadeMusic, SoundID: KDMainTime, Bus: sound.BusMusic, FadeMS: MainTimeFadeMS}
	if prevOn && !curOn {
		return []Cue{fade}
	}
	if kdFinished(cur) && !kdFinished(prev) {
		return []Cue{fade}
	}
	if kdQuestionShown(cur) && (!kdQuestionShown(prev) ||
		prev.Controller.SelectedQuestionID != cur.Controller.SelectedQuestionID) {
		return []Cue{{Op: OpPlayIfIdle, SoundID: KDMainTime, Bus: sound.BusMusic}}
	}
	return nil
}

func timerRunning(timer game.TimerState) bool { return timer.IsRunning && !timer.IsPaused }

func timerCues(prev, cur game.RuntimeState) []Cue {
	was, is := timerRunning(prev.Timer), timerRunning(cur.Timer)
	stop := Cue{Op: OpStopBus, Bus: sound.BusTimer}
	if is {
		segment := !was || prev.Timer.StartedAt != cur.Timer.StartedAt ||
			prev.Timer.DurationMS != cur.Timer.DurationMS || prev.Timer.Kind != cur.Timer.Kind
		if !segment {
			return nil
		}
		id := TimerSound(cur)
		if id == "" {
			if was {
				return []Cue{stop}
			}
			return nil
		}
		value := play(id, sound.BusTimer)
		// Fresh start: elapsed 0. Resume: continue at the elapsed offset.
		value.StartAt = cur.Timer.StartedAt - cur.Timer.ElapsedMS
		return []Cue{value}
	}
	if !was {
		return nil
	}
	naturalEnd := !cur.Timer.IsPaused && cur.Timer.DurationMS > 0 && cur.Timer.RemainingMS <= 0 &&
		cur.Timer.DurationMS == prev.Timer.DurationMS
	if naturalEnd {
		return nil
	}
	// Pause, reset, or a screen/question change froze the timer early.
	return []Cue{stop}
}

// TimerSound picks the timer-bus sound for the running timer ("" if none).
func TimerSound(state game.RuntimeState) string {
	timer := state.Timer
	switch timer.Kind {
	case "steal":
		return VD5Seconds
	case "obstacle":
		return BonusThinking
	}
	switch state.Controller.Phase {
	case controller.PhaseKhoiDong:
		return KD3Seconds
	case controller.PhaseVCNV:
		return VCNV15Seconds
	case controller.PhaseTangToc:
		return nearest(timer.DurationMS, []int64{10_000, 20_000, 30_000, 40_000}, []string{TT10Seconds, TT20Seconds, TT30Seconds, TT40Seconds})
	case controller.PhaseVeDich:
		if timer.DurationMS == 5_000 {
			return VD5Seconds
		}
		return nearest(timer.DurationMS, []int64{15_000, 20_000}, []string{VD15Seconds, VD20Seconds})
	}
	return ""
}

func nearest(duration int64, durations []int64, ids []string) string {
	best, bestDistance := "", int64(-1)
	for i, candidate := range durations {
		distance := candidate - duration
		if distance < 0 {
			distance = -distance
		}
		if bestDistance < 0 || distance < bestDistance {
			best, bestDistance = ids[i], distance
		}
	}
	return best
}

func hasClaim(state game.RuntimeState) bool {
	for _, contestant := range state.Contestants {
		if contestant.HasRung {
			return true
		}
	}
	return false
}

func rang(prev game.RuntimeState, slot int) bool {
	for _, contestant := range prev.Contestants {
		if contestant.Slot == slot {
			return contestant.HasRung
		}
	}
	return false
}

func bellCues(prev, cur game.RuntimeState, settings Settings) []Cue {
	if prev.Controller.Phase != cur.Controller.Phase {
		return nil
	}
	switch cur.Controller.Phase {
	case controller.PhaseVCNV:
		var cues []Cue
		for _, contestant := range cur.Contestants {
			if contestant.HasRung && !rang(prev, contestant.Slot) {
				cues = append(cues, effect(VCNVObstacleGrant))
			}
		}
		return cues
	case controller.PhaseKhoiDong:
		if hasClaim(cur) && !hasClaim(prev) {
			id := settings.BellSoundID
			if id == "" {
				id = KDBell
			}
			return []Cue{effect(id)}
		}
	case controller.PhaseVeDich:
		if hasClaim(cur) && !hasClaim(prev) {
			return []Cue{effect(VDGrant)}
		}
	}
	return nil
}

func vcnvCues(prev, cur game.RuntimeState) []Cue {
	if prev.Controller.Phase != controller.PhaseVCNV || cur.Controller.Phase != controller.PhaseVCNV {
		return nil
	}
	var cues []Cue
	previous, current := prev.Controller.VCNVState, cur.Controller.VCNVState
	if current.HasStarted && !previous.HasStarted {
		cues = append(cues, effect(VCNVRowsShow))
	}
	for i, state := range current.RowStates {
		if state == "s" && (i >= len(previous.RowStates) || previous.RowStates[i] != "s") {
			cues = append(cues, effect(VCNVRowChoose))
			break
		}
	}
	for i, open := range current.ImageOpenState {
		if open && (i >= len(previous.ImageOpenState) || !previous.ImageOpenState[i]) {
			cues = append(cues, effect(VCNVPictureReveal))
			break
		}
	}
	return cues
}

func veDichCues(prev, cur game.RuntimeState) []Cue {
	if prev.Controller.Phase != controller.PhaseVeDich || cur.Controller.Phase != controller.PhaseVeDich {
		return nil
	}
	var cues []Cue
	previous, current := prev.Controller.VeDichContestantsState, cur.Controller.VeDichContestantsState
	confirmed, starred := false, false
	for i, contestant := range current {
		var before controller.VeDichContestantState
		if i < len(previous) {
			before = previous[i]
		}
		if len(contestant.SelectedQuestionSet) == 3 && !slices.Equal(before.SelectedQuestionSet, contestant.SelectedQuestionSet) {
			confirmed = true
		}
		if contestant.StarredOn != 0 && contestant.StarredOn != before.StarredOn {
			starred = true
		}
	}
	if confirmed {
		for i := range 3 {
			cues = append(cues, delayed(VDChoiceChosen, int64(i*VDChoiceIntervalMS)))
		}
	}
	if starred {
		cues = append(cues, effect(VDStarChoose))
	}
	return cues
}
