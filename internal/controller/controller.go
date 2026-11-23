package controller

import (
	"fmt"

	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
)

type Phase string
type Screen string

const (
	PhaseIdle     Phase = "idle"
	PhaseMain     Phase = "main"
	PhaseScore    Phase = "score"
	PhaseKhoiDong Phase = "khoi_dong"
	PhaseVCNV     Phase = "vcnv"
	PhaseTangToc  Phase = "tang_toc"
	PhaseVeDich   Phase = "ve_dich"
	// PhaseLogo is the legacy audience logo screen.
	PhaseLogo Phase = "logo"
)

const (
	ScreenIntro       Screen = "intro"
	ScreenMain        Screen = "main"
	ScreenScore       Screen = "score"
	ScreenStart       Screen = "start"
	ScreenRule        Screen = "rule"
	ScreenMainKD      Screen = "main_kd"
	ScreenQuesChung   Screen = "ques_chung"
	ScreenQuesTS1     Screen = "ques_ts1"
	ScreenQuesTS2     Screen = "ques_ts2"
	ScreenQuesTS3     Screen = "ques_ts3"
	ScreenQuesTS4     Screen = "ques_ts4"
	ScreenEndTS       Screen = "end_ts"
	ScreenEnd         Screen = "end"
	ScreenMainVCNV    Screen = "main_vcnv"
	ScreenImageVCNV   Screen = "image_vcnv"
	ScreenQues        Screen = "ques"
	ScreenSolve       Screen = "solve"
	ScreenAnswersTT   Screen = "answers_tt"
	ScreenAnswersVCNV Screen = "answers_vcnv"
	ScreenMainVD      Screen = "main_vd"
	ScreenPreQuesTS1  Screen = "pre_ques_ts1"
	ScreenPreQuesTS2  Screen = "pre_ques_ts2"
	ScreenPreQuesTS3  Screen = "pre_ques_ts3"
	ScreenPreQuesTS4  Screen = "pre_ques_ts4"
	// ScreenTestBell is the Khởi động bell check before the shared turn.
	ScreenTestBell Screen = "test_bell"
)

var screenCatalog = map[Phase][]Screen{
	PhaseIdle:     {ScreenIntro},
	PhaseLogo:     {ScreenIntro},
	PhaseMain:     {ScreenMain},
	PhaseScore:    {ScreenScore},
	PhaseKhoiDong: {ScreenStart, ScreenRule, ScreenIntro, ScreenMainKD, ScreenTestBell, ScreenQuesChung, ScreenQuesTS1, ScreenQuesTS2, ScreenQuesTS3, ScreenQuesTS4, ScreenEndTS, ScreenEnd},
	PhaseVCNV:     {ScreenStart, ScreenRule, ScreenIntro, ScreenMainVCNV, ScreenImageVCNV, ScreenQues, ScreenAnswersVCNV, ScreenEnd},
	PhaseTangToc:  {ScreenStart, ScreenRule, ScreenIntro, ScreenQues, ScreenAnswersTT, ScreenSolve, ScreenEnd},
	PhaseVeDich:   {ScreenStart, ScreenRule, ScreenIntro, ScreenMainVD, ScreenPreQuesTS1, ScreenQuesTS1, ScreenPreQuesTS2, ScreenQuesTS2, ScreenPreQuesTS3, ScreenQuesTS3, ScreenPreQuesTS4, ScreenQuesTS4, ScreenEndTS, ScreenEnd},
}

func Screens(phase Phase) []Screen {
	return append([]Screen(nil), screenCatalog[phase]...)
}

func DefaultScreen(phase Phase) Screen {
	values := screenCatalog[phase]
	if len(values) == 0 {
		return ScreenIntro
	}
	return values[0]
}

func validScreen(phase Phase, screen Screen) bool {
	for _, candidate := range screenCatalog[phase] {
		if candidate == screen {
			return true
		}
	}
	return false
}

func IsQuestionScreen(screen Screen) bool {
	return screen == ScreenQues || screen == ScreenSolve || screen == ScreenAnswersTT || screen == ScreenAnswersVCNV || screen == ScreenQuesChung || screen == ScreenQuesTS1 || screen == ScreenQuesTS2 || screen == ScreenQuesTS3 || screen == ScreenQuesTS4
}

func IsRound(phase Phase) bool {
	return phase == PhaseKhoiDong || phase == PhaseVCNV || phase == PhaseTangToc || phase == PhaseVeDich
}

type VCNVState struct {
	HasStarted     bool     `json:"has_started"`
	HasSolved      bool     `json:"has_solved"`
	RowStates      []string `json:"row_states"`
	ImageOpenState []bool   `json:"image_open_state"`
}

type VeDichContestantState struct {
	SelectedQuestionSet []string `json:"selected_question_set"`
	StarredOn           int      `json:"starred_on"`
}
type VeDichVideoStatus string

const (
	VeDichVideoIdle    VeDichVideoStatus = "idle"
	VeDichVideoPlaying VeDichVideoStatus = "playing"
	VeDichVideoPaused  VeDichVideoStatus = "paused"
	VeDichVideoHolding VeDichVideoStatus = "holding"
)

type VeDichVideoState struct {
	SessionID  uint64            `json:"session_id"`
	QuestionID string            `json:"question_id,omitempty"`
	Status     VeDichVideoStatus `json:"status"`
	Volume     float64           `json:"volume"`
	// StartedAt is the server time (ms) when the current play segment started;
	// 0 unless playing. PositionMS is the media position at segment start (or
	// where it was paused/stopped). DurationMS is 0 until a client reports it.
	StartedAt  int64 `json:"started_at"`
	PositionMS int64 `json:"position_ms"`
	DurationMS int64 `json:"duration_ms"`
}

type State struct {
	Phase                  Phase                   `json:"phase"`
	Screen                 Screen                  `json:"screen"`
	SelectedQuestionID     string                  `json:"selected_question_id"`
	SelectedContestant     int                     `json:"selected_contestant"`
	IsAllowingBell         bool                    `json:"is_allowing_bell"`
	IsShowingQuestion      bool                    `json:"is_showing_question"`
	AnswerStatus           []bool                  `json:"answer_status"`
	VCNVState              VCNVState               `json:"vcnv_state"`
	VeDichContestantsState []VeDichContestantState `json:"ve_dich_contestants_state"`
	VeDichVideo            VeDichVideoState        `json:"ve_dich_video"`
}

func validation(field, message string) error {
	return &opsocket.Error{Code: "VALIDATION", Message: message, Details: map[string]any{"field": field}}
}

func AnswerVisible(state State) bool {
	switch state.Phase {
	case PhaseTangToc:
		return state.Screen == ScreenSolve
	case PhaseVCNV:
		return state.VCNVState.HasSolved
	case PhaseKhoiDong, PhaseVeDich:
		return state.Screen == ScreenEndTS || state.Screen == ScreenEnd
	default:
		return false
	}
}

func ContestantAnswersVisible(state State) bool {
	return state.Phase == PhaseTangToc && state.Screen == ScreenAnswersTT ||
		state.Phase == PhaseVCNV && state.Screen == ScreenAnswersVCNV
}

// ValidateState checks the complete controller contract before it can enter runtime.
func ValidateState(state State) error {
	if _, ok := screenCatalog[state.Phase]; !ok {
		return validation("phase", "unknown phase")
	}
	if !validScreen(state.Phase, state.Screen) {
		return validation("screen", fmt.Sprintf("screen %q is not valid for phase %q", state.Screen, state.Phase))
	}
	if state.SelectedContestant < 0 || state.SelectedContestant > 4 {
		return validation("selected_contestant", "contestant index must be between 0 and 4")
	}
	if len(state.AnswerStatus) != 0 && len(state.AnswerStatus) != 4 {
		return validation("answer_status", "answer status must contain exactly four entries")
	}
	if len(state.VCNVState.RowStates) != 4 {
		return validation("vcnv_state.row_states", "VCNV must contain exactly four row states")
	}
	for i, rowState := range state.VCNVState.RowStates {
		if rowState != "u" && rowState != "s" && rowState != "c" && rowState != "w" {
			return validation(fmt.Sprintf("vcnv_state.row_states[%d]", i), "VCNV row state must be u, s, c, or w")
		}
	}
	if len(state.VCNVState.ImageOpenState) != 5 {
		return validation("vcnv_state.image_open_state", "VCNV must contain exactly five image panels")
	}
	if len(state.VeDichContestantsState) != 4 {
		return validation("ve_dich_contestants_state", "Về đích must contain exactly four contestant states")
	}
	for i, contestant := range state.VeDichContestantsState {
		if contestant.StarredOn < 0 || contestant.StarredOn > 3 {
			return validation(fmt.Sprintf("ve_dich_contestants_state[%d].starred_on", i), "starred question must be between 0 and 3")
		}
		if len(contestant.SelectedQuestionSet) != 0 && len(contestant.SelectedQuestionSet) != 3 {
			return validation(fmt.Sprintf("ve_dich_contestants_state[%d].selected_question_set", i), "question package must contain zero or three questions")
		}
		for question, choice := range contestant.SelectedQuestionSet {
			if !validVDChoice(choice) {
				return validation(fmt.Sprintf("ve_dich_contestants_state[%d].selected_question_set[%d]", i, question), "question package choice must be 20_1 through 30_3")
			}
		}
	}
	video := state.VeDichVideo
	if video.Status != "" && video.Status != VeDichVideoIdle && video.Status != VeDichVideoPlaying && video.Status != VeDichVideoPaused && video.Status != VeDichVideoHolding {
		return validation("ve_dich_video.status", "video status must be idle, playing, paused, or holding")
	}
	if video.StartedAt < 0 || video.PositionMS < 0 || video.DurationMS < 0 {
		return validation("ve_dich_video", "video clock values must not be negative")
	}
	if video.Volume < 0 || video.Volume > 1 {
		return validation("ve_dich_video.volume", "video volume must be between zero and one")
	}
	if video.Status != "" && video.Status != VeDichVideoIdle && (video.SessionID == 0 || video.QuestionID == "") {
		return validation("ve_dich_video", "active video state needs a session and question ID")
	}
	return nil
}

func validVDChoice(value string) bool {
	if len(value) != 4 || (value[:3] != "20_" && value[:3] != "30_") {
		return false
	}
	return value[3] >= '1' && value[3] <= '3'
}
func DefaultState() State {
	return State{
		Phase: PhaseIdle, Screen: ScreenIntro, AnswerStatus: []bool{true, true, true, true},
		VCNVState:              VCNVState{RowStates: []string{"u", "u", "u", "u"}, ImageOpenState: []bool{false, false, false, false, false}},
		VeDichContestantsState: []VeDichContestantState{{SelectedQuestionSet: []string{}}, {SelectedQuestionSet: []string{}}, {SelectedQuestionSet: []string{}}, {SelectedQuestionSet: []string{}}},
		VeDichVideo:            VeDichVideoState{Status: VeDichVideoIdle, Volume: 1},
	}
}
