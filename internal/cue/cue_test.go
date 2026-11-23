package cue

import (
	"reflect"
	"testing"

	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/sound"
)

var enabled = Settings{Enabled: true, BellSoundID: KDBell}

func state(phase controller.Phase, screen controller.Screen) game.RuntimeState {
	value := game.RuntimeState{MatchID: "match", Epoch: 3, Status: game.StatusPlaying, Controller: controller.DefaultState()}
	value.Controller.Phase, value.Controller.Screen = phase, screen
	for slot := 1; slot <= 4; slot++ {
		value.Contestants = append(value.Contestants, game.ContestantState{Slot: slot})
	}
	return value
}

func with(value game.RuntimeState, mutate func(*game.RuntimeState)) game.RuntimeState {
	value.Controller.AnswerStatus = append([]bool(nil), value.Controller.AnswerStatus...)
	value.Controller.VCNVState.RowStates = append([]string(nil), value.Controller.VCNVState.RowStates...)
	value.Controller.VCNVState.ImageOpenState = append([]bool(nil), value.Controller.VCNVState.ImageOpenState...)
	vd := make([]controller.VeDichContestantState, len(value.Controller.VeDichContestantsState))
	for i, contestant := range value.Controller.VeDichContestantsState {
		vd[i] = contestant
		vd[i].SelectedQuestionSet = append([]string(nil), contestant.SelectedQuestionSet...)
	}
	value.Controller.VeDichContestantsState = vd
	value.Contestants = append([]game.ContestantState(nil), value.Contestants...)
	mutate(&value)
	return value
}

func showing(id string) func(*game.RuntimeState) {
	return func(s *game.RuntimeState) {
		s.Controller.SelectedQuestionID = id
		s.Controller.IsShowingQuestion = true
	}
}

func running(duration, startedAt int64, kind string) func(*game.RuntimeState) {
	return func(s *game.RuntimeState) {
		s.Timer = game.TimerState{IsRunning: true, DurationMS: duration, RemainingMS: duration, StartedAt: startedAt, Kind: kind}
	}
}

func ring(slots ...int) func(*game.RuntimeState) {
	return func(s *game.RuntimeState) {
		for _, slot := range slots {
			s.Contestants[slot-1].HasRung = true
		}
	}
}

func timerPlay(id string, startAt int64) Cue {
	return Cue{Op: OpPlay, SoundID: id, Bus: sound.BusTimer, StartAt: startAt}
}

var (
	stopTimer = Cue{Op: OpStopBus, Bus: sound.BusTimer}
	fadeMain  = Cue{Op: OpFadeMusic, SoundID: KDMainTime, Bus: sound.BusMusic, FadeMS: 700}
	playMain  = Cue{Op: OpPlayIfIdle, SoundID: KDMainTime, Bus: sound.BusMusic}
)

func TestCuesTable(t *testing.T) {
	const (
		kd   = controller.PhaseKhoiDong
		vcnv = controller.PhaseVCNV
		tt   = controller.PhaseTangToc
		vd   = controller.PhaseVeDich
	)
	kdQ1 := with(state(kd, controller.ScreenQuesTS1), showing("kd_rieng.0.0"))
	kdChung := state(kd, controller.ScreenQuesChung)
	vcnvBoard := state(vcnv, controller.ScreenMainVCNV)
	vcnvQues := with(state(vcnv, controller.ScreenQues), showing("vcnv.rows.0"))
	ttQues := with(state(tt, controller.ScreenQues), showing("tt.0"))
	vdQues := with(state(vd, controller.ScreenQuesTS2), showing("vd.1.20.0"))
	vdPre := state(vd, controller.ScreenPreQuesTS2)

	cases := []struct {
		name      string
		prev, cur game.RuntimeState
		want      []Cue
	}{
		// Bắt đầu
		{"KĐ start", state(controller.PhaseMain, controller.ScreenMain), state(kd, controller.ScreenStart), []Cue{effect(KDStartRound)}},
		{"VCNV start", state(kd, controller.ScreenEnd), state(vcnv, controller.ScreenStart), []Cue{effect(MainStart)}},
		{"TT start", state(tt, controller.ScreenIntro), state(tt, controller.ScreenStart), []Cue{effect(MainStart)}},
		{"VĐ start", state(vd, controller.ScreenRule), state(vd, controller.ScreenStart), []Cue{effect(MainStart)}},
		// Intro
		{"KĐ intro", state(kd, controller.ScreenStart), state(kd, controller.ScreenIntro), []Cue{effect(KDStartTurn)}},
		{"VCNV intro", state(vcnv, controller.ScreenStart), state(vcnv, controller.ScreenIntro), []Cue{effect(VCNVStartRound)}},
		{"TT intro", state(tt, controller.ScreenStart), state(tt, controller.ScreenIntro), []Cue{effect(TTStartRound)}},
		{"VĐ intro", state(vd, controller.ScreenStart), state(vd, controller.ScreenIntro), []Cue{effect(VDStartRound)}},
		// Silent screens
		{"rule silent", state(kd, controller.ScreenStart), state(kd, controller.ScreenRule), nil},
		{"idle silent", state(controller.PhaseMain, controller.ScreenMain), state(controller.PhaseIdle, controller.ScreenIntro), nil},
		{"logo silent", state(controller.PhaseIdle, controller.ScreenIntro), state(controller.PhaseLogo, controller.ScreenIntro), nil},
		{"main silent", state(controller.PhaseIdle, controller.ScreenIntro), state(controller.PhaseMain, controller.ScreenMain), nil},
		{"score silent", state(controller.PhaseMain, controller.ScreenMain), state(controller.PhaseScore, controller.ScreenScore), nil},
		{"main_kd silent", state(kd, controller.ScreenIntro), state(kd, controller.ScreenMainKD), nil},
		{"test_bell silent", state(kd, controller.ScreenMainKD), state(kd, controller.ScreenTestBell), nil},
		{"main_vcnv silent", state(vcnv, controller.ScreenIntro), state(vcnv, controller.ScreenMainVCNV), nil},
		{"image_vcnv silent", vcnvBoard, state(vcnv, controller.ScreenImageVCNV), nil},
		{"TT solve silent", state(tt, controller.ScreenAnswersTT), state(tt, controller.ScreenSolve), nil},
		{"main_vd silent", state(vd, controller.ScreenIntro), state(vd, controller.ScreenMainVD), nil},
		{"same screen silent", state(kd, controller.ScreenIntro), state(kd, controller.ScreenIntro), nil},
		// KĐ question screens
		{"KĐ ques_chung from main_kd", state(kd, controller.ScreenMainKD), kdChung,
			[]Cue{effect(KDStartTurn), delayed(KDPreMainTime, 4500)}},
		{"KĐ ques_ts1 from test_bell", state(kd, controller.ScreenTestBell), state(kd, controller.ScreenQuesTS1),
			[]Cue{effect(KDStartTurn), delayed(KDPreMainTime, 4500)}},
		{"KĐ ques_ts1 from ques_chung: no cue", kdChung, state(kd, controller.ScreenQuesTS1), nil},
		{"KĐ ques_ts2 from ques_ts1: no cue", state(kd, controller.ScreenQuesTS1), state(kd, controller.ScreenQuesTS2), nil},
		{"VĐ ques_ts1 is not a KĐ question screen", state(kd, controller.ScreenQuesTS1), state(vd, controller.ScreenQuesTS1),
			[]Cue{effect(VDPackageChoose), fadeMain}},
		// MainTime
		{"KĐ question shown", state(kd, controller.ScreenQuesTS1), kdQ1, []Cue{playMain}},
		{"KĐ next question auto-shown", kdQ1, with(kdQ1, showing("kd_rieng.0.1")), []Cue{playMain}},
		{"KĐ question shown off question screen: none", state(kd, controller.ScreenMainKD), with(state(kd, controller.ScreenMainKD), showing("kd_chung.0")), nil},
		{"KĐ KẾT THÚC fades", kdQ1, with(kdQ1, showing("")), []Cue{fadeMain}},
		{"KĐ leave to end_ts fades", kdQ1, state(kd, controller.ScreenEndTS), []Cue{effect(KDFinishTurn), fadeMain}},
		{"KĐ leave to main_kd fades", kdQ1, state(kd, controller.ScreenMainKD), []Cue{fadeMain}},
		{"KĐ leave phase fades", kdQ1, state(vcnv, controller.ScreenStart), []Cue{effect(MainStart), fadeMain}},
		// KĐ timer and bell
		{"KĐ timer", kdQ1, with(kdQ1, running(3000, 1000, "question")), []Cue{timerPlay(KD3Seconds, 1000)}},
		{"KĐ bell first claim", with(kdChung, showing("kd_chung.0")), with(with(kdChung, showing("kd_chung.0")), ring(2)), []Cue{effect(KDBell)}},
		{"KĐ bell second ring silent", with(kdChung, ring(2)), with(kdChung, ring(2, 3)), nil},
		{"KĐ test_bell ring", state(kd, controller.ScreenTestBell), with(state(kd, controller.ScreenTestBell), ring(4)), []Cue{effect(KDBell)}},
		{"KĐ test_bell after clear", state(kd, controller.ScreenTestBell), with(state(kd, controller.ScreenTestBell), ring(1)), []Cue{effect(KDBell)}},
		// end_ts / end
		{"VĐ end_ts", vdQues, state(vd, controller.ScreenEndTS), []Cue{effect(KDFinishTurn)}},
		{"KĐ end", state(kd, controller.ScreenEndTS), state(kd, controller.ScreenEnd), []Cue{effect(VDFinishRound)}},
		{"VCNV end", vcnvBoard, state(vcnv, controller.ScreenEnd), []Cue{effect(VDFinishRound)}},
		{"TT end", state(tt, controller.ScreenSolve), state(tt, controller.ScreenEnd), []Cue{effect(VDFinishRound)}},
		{"VĐ end", state(vd, controller.ScreenEndTS), state(vd, controller.ScreenEnd), []Cue{effect(VDFinishRound)}},
		// VCNV
		{"VCNV rows show", vcnvBoard, with(vcnvBoard, func(s *game.RuntimeState) { s.Controller.VCNVState.HasStarted = true }), []Cue{effect(VCNVRowsShow)}},
		{"VCNV row choose", vcnvBoard, with(vcnvBoard, func(s *game.RuntimeState) { s.Controller.VCNVState.RowStates[2] = "s" }), []Cue{effect(VCNVRowChoose)}},
		{"VCNV row s→c silent", with(vcnvBoard, func(s *game.RuntimeState) { s.Controller.VCNVState.RowStates[2] = "s" }), with(vcnvBoard, func(s *game.RuntimeState) { s.Controller.VCNVState.RowStates[2] = "c" }), nil},
		{"VCNV ques entered", vcnvBoard, state(vcnv, controller.ScreenQues), []Cue{effect(VCNVRowQuestShow)}},
		{"VCNV row timer", vcnvQues, with(vcnvQues, running(15000, 50, "question")), []Cue{timerPlay(VCNV15Seconds, 50)}},
		{"VCNV centre timer", with(vcnvQues, showing("vcnv.center")), with(with(vcnvQues, showing("vcnv.center")), running(30000, 50, "question")), []Cue{timerPlay(VCNV15Seconds, 50)}},
		{"VCNV obstacle timer", vcnvBoard, with(vcnvBoard, running(15000, 70, "obstacle")), []Cue{timerPlay(BonusThinking, 70)}},
		{"VCNV tile opened", vcnvBoard, with(vcnvBoard, func(s *game.RuntimeState) { s.Controller.VCNVState.ImageOpenState[4] = true }), []Cue{effect(VCNVPictureReveal)}},
		{"VCNV tile closed silent", with(vcnvBoard, func(s *game.RuntimeState) { s.Controller.VCNVState.ImageOpenState[4] = true }), vcnvBoard, nil},
		{"VCNV bell", vcnvQues, with(vcnvQues, ring(1)), []Cue{effect(VCNVObstacleGrant)}},
		{"VCNV bell every ring", with(vcnvQues, ring(1)), with(vcnvQues, ring(1, 3)), []Cue{effect(VCNVObstacleGrant)}},
		{"VCNV answers", vcnvQues, state(vcnv, controller.ScreenAnswersVCNV), []Cue{effect(VCNVAnswersShowing)}},
		// TT
		{"TT answers", ttQues, state(tt, controller.ScreenAnswersTT), []Cue{effect(TTAnswersShowing)}},
		{"TT ques entered", state(tt, controller.ScreenSolve), state(tt, controller.ScreenQues), []Cue{effect(TTQuestionShowing)}},
		{"TT 10 s", ttQues, with(ttQues, running(10000, 9, "question")), []Cue{timerPlay(TT10Seconds, 9)}},
		{"TT 20 s", ttQues, with(ttQues, running(20000, 9, "question")), []Cue{timerPlay(TT20Seconds, 9)}},
		{"TT 30 s", ttQues, with(ttQues, running(30000, 9, "question")), []Cue{timerPlay(TT30Seconds, 9)}},
		{"TT 40 s", ttQues, with(ttQues, running(40000, 9, "question")), []Cue{timerPlay(TT40Seconds, 9)}},
		// VĐ
		{"VĐ pre_ques entered", state(vd, controller.ScreenMainVD), vdPre, []Cue{effect(VDStartTurn), delayed(VDPackageShow, 5000)}},
		{"VĐ pre_ques within group silent", state(vd, controller.ScreenPreQuesTS1), vdPre, nil},
		{"VĐ package confirmed", vdPre, with(vdPre, func(s *game.RuntimeState) {
			s.Controller.VeDichContestantsState[1].SelectedQuestionSet = []string{"20_1", "20_2", "30_3"}
		}), []Cue{effect(VDChoiceChosen), delayed(VDChoiceChosen, 500), delayed(VDChoiceChosen, 1000)}},
		{"VĐ ques entered", vdPre, state(vd, controller.ScreenQuesTS2), []Cue{effect(VDPackageChoose)}},
		{"VĐ ques within group silent", state(vd, controller.ScreenQuesTS1), state(vd, controller.ScreenQuesTS2), nil},
		{"VĐ star", vdQues, with(vdQues, func(s *game.RuntimeState) { s.Controller.VeDichContestantsState[1].StarredOn = 2 }), []Cue{effect(VDStarChoose)}},
		{"VĐ unstar silent", with(vdQues, func(s *game.RuntimeState) { s.Controller.VeDichContestantsState[1].StarredOn = 2 }), vdQues, nil},
		{"VĐ 15 s", vdQues, with(vdQues, running(15000, 5, "question")), []Cue{timerPlay(VD15Seconds, 5)}},
		{"VĐ 20 s", vdQues, with(vdQues, running(20000, 5, "question")), []Cue{timerPlay(VD20Seconds, 5)}},
		{"VĐ steal", vdQues, with(vdQues, running(5000, 6, "steal")), []Cue{timerPlay(VD5Seconds, 6)}},
		{"VĐ steal replaces running owner timer", with(vdQues, running(15000, 5, "question")), with(vdQues, running(5000, 6, "steal")), []Cue{timerPlay(VD5Seconds, 6)}},
		{"VĐ bell first claim", with(vdQues, running(5000, 6, "steal")), with(with(vdQues, running(5000, 6, "steal")), ring(3)), []Cue{effect(VDGrant)}},
		// Timer channel
		{"timer still running: nothing", with(ttQues, running(30000, 9, "question")), with(ttQues, running(30000, 9, "question")), nil},
		{"timer pause stops", with(ttQues, running(30000, 9, "question")), with(ttQues, func(s *game.RuntimeState) {
			s.Timer = game.TimerState{IsRunning: true, IsPaused: true, DurationMS: 30000, RemainingMS: 21000, ElapsedMS: 9000, Kind: "question"}
		}), []Cue{stopTimer}},
		{"timer resume continues at offset", with(ttQues, func(s *game.RuntimeState) {
			s.Timer = game.TimerState{IsRunning: true, IsPaused: true, DurationMS: 30000, RemainingMS: 21000, ElapsedMS: 9000, Kind: "question"}
		}), with(ttQues, func(s *game.RuntimeState) {
			s.Timer = game.TimerState{IsRunning: true, DurationMS: 30000, RemainingMS: 21000, ElapsedMS: 9000, StartedAt: 100000, Kind: "question"}
		}), []Cue{timerPlay(TT30Seconds, 91000)}},
		{"timer reset stops", with(ttQues, running(30000, 9, "question")), ttQues, []Cue{stopTimer}},
		{"timer reset while paused: nothing", with(ttQues, func(s *game.RuntimeState) {
			s.Timer = game.TimerState{IsRunning: true, IsPaused: true, DurationMS: 30000, RemainingMS: 21000, ElapsedMS: 9000}
		}), ttQues, nil},
		{"timer frozen by screen change stops", with(ttQues, running(30000, 9, "question")), with(state(tt, controller.ScreenAnswersTT), func(s *game.RuntimeState) {
			s.Timer = game.TimerState{DurationMS: 30000, RemainingMS: 12000, ElapsedMS: 18000, Kind: "question"}
		}), []Cue{effect(TTAnswersShowing), stopTimer}},
		{"timer stopped by question change", with(kdQ1, running(3000, 1, "question")), with(kdQ1, showing("kd_rieng.0.1")), []Cue{playMain, stopTimer}},
		{"timer natural end: nothing", with(ttQues, running(30000, 9, "question")), with(ttQues, func(s *game.RuntimeState) {
			s.Timer = game.TimerState{DurationMS: 30000, ElapsedMS: 30000, Kind: "question"}
		}), nil},
		// Baseline
		{"baseline: previous zero", game.RuntimeState{}, state(kd, controller.ScreenStart), nil},
		{"baseline: match changed", with(state(kd, controller.ScreenIntro), func(s *game.RuntimeState) { s.MatchID = "other" }), state(kd, controller.ScreenStart), nil},
		{"baseline: epoch changed", with(state(kd, controller.ScreenIntro), func(s *game.RuntimeState) { s.Epoch = 2 }), state(kd, controller.ScreenStart), nil},
		{"baseline: unloaded", state(kd, controller.ScreenIntro), game.RuntimeState{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Cues(tc.prev, tc.cur, enabled)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("cues\n got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestCustomBellAndDisabled(t *testing.T) {
	prev := state(controller.PhaseKhoiDong, controller.ScreenTestBell)
	cur := with(prev, ring(1))
	got := Cues(prev, cur, Settings{Enabled: true, BellSoundID: "upload.x"})
	if !reflect.DeepEqual(got, []Cue{effect("upload.x")}) {
		t.Fatalf("custom bell: %+v", got)
	}
	if got := Cues(prev, cur, Settings{Enabled: false, BellSoundID: KDBell}); len(got) != 0 {
		t.Fatalf("sound disabled must not cue: %+v", got)
	}
}

func TestBuiltinBuses(t *testing.T) {
	buses := map[string]sound.Bus{}
	for _, value := range sound.Builtins() {
		buses[value.ID] = value.Bus
		if value.ID == KDMainTime && !value.Loop {
			t.Fatal("MainTime must loop")
		}
	}
	cues := []Cue{
		effect(MainStart), effect(KDStartRound), effect(KDStartTurn), effect(KDPreMainTime), effect(KDFinishTurn), effect(KDBell),
		effect(VCNVStartRound), effect(VCNVRowsShow), effect(VCNVRowChoose), effect(VCNVRowQuestShow), effect(VCNVPictureReveal),
		effect(VCNVObstacleGrant), effect(VCNVAnswersShowing), effect(TTStartRound), effect(TTQuestionShowing), effect(TTAnswersShowing),
		effect(VDStartRound), effect(VDStartTurn), effect(VDPackageShow), effect(VDChoiceChosen), effect(VDPackageChoose),
		effect(VDStarChoose), effect(VDGrant), effect(VDFinishRound), playMain,
		timerPlay(KD3Seconds, 0), timerPlay(VCNV15Seconds, 0), timerPlay(BonusThinking, 0), timerPlay(TT10Seconds, 0),
		timerPlay(TT20Seconds, 0), timerPlay(TT30Seconds, 0), timerPlay(TT40Seconds, 0), timerPlay(VD5Seconds, 0),
		timerPlay(VD15Seconds, 0), timerPlay(VD20Seconds, 0),
	}
	for _, value := range cues {
		bus, ok := buses[value.SoundID]
		if !ok {
			t.Fatalf("%s is not a built-in sound", value.SoundID)
		}
		if bus != value.Bus {
			t.Fatalf("%s: built-in bus %s, cue bus %s", value.SoundID, bus, value.Bus)
		}
	}
	for _, id := range []string{"builtin.vd.practice20seconds", "builtin.vd.practice60seconds"} {
		if buses[id] != sound.BusTimer {
			t.Fatalf("%s should be on the timer bus", id)
		}
	}
	if buses["builtin.main.pointsummary"] != sound.BusEffects {
		t.Fatal("other sounds are effects")
	}
}
