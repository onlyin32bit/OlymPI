package game_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
	store "github.com/onlyin32bit/4t/internal/store/sqlite"
)

func TestSelectedMatchLifecycleAndSwitchGuard(t *testing.T) {
	ctx := context.Background()
	service, closeStore := newService(t, ctx)
	defer closeStore()
	set, err := service.SaveQuestionSet(ctx, "", game.SaveQuestionSetInput{Name: "Đề 1", Data: validQuestions()}, game.Actor{UserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.CreateMatch(ctx, matchInput("Trận 1", set.ID), game.Actor{UserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateMatch(ctx, matchInput("Trận 2", set.ID), game.Actor{UserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.SelectMatch(ctx, first.ID, game.Actor{UserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	state, err = service.Transition(ctx, game.StatusPlaying, command(state), game.Actor{UserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SelectMatch(ctx, second.ID, game.Actor{UserID: 1}); err == nil {
		t.Fatal("expected switching a playing match to fail")
	}
	state, err = service.Transition(ctx, game.StatusPaused, command(state), game.Actor{UserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	selected, err := service.SelectMatch(ctx, second.ID, game.Actor{UserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if selected.MatchID != second.ID || selected.Epoch <= state.Epoch {
		t.Fatalf("unexpected selected state: %+v", selected)
	}
	firstEvents, err := service.ListEvents(ctx, first.ID)
	if err != nil || len(firstEvents) == 0 || firstEvents[len(firstEvents)-1].Type != "Match.Unloaded" {
		t.Fatalf("first match did not record unload: %+v, %v", firstEvents, err)
	}
	secondEvents, err := service.ListEvents(ctx, second.ID)
	if err != nil || len(secondEvents) == 0 || secondEvents[len(secondEvents)-1].Type != "Match.Loaded" {
		t.Fatalf("second match did not record load: %+v, %v", secondEvents, err)
	}
	if _, err = service.UnloadMatch(ctx, command(selected), game.Actor{UserID: 1}); err != nil {
		t.Fatal(err)
	}
	listed, err := service.ListMatches(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range listed {
		if match.Selected {
			t.Fatalf("match remained selected after unload: %+v", match)
		}
	}
	secondEvents, err = service.ListEvents(ctx, second.ID)
	if err != nil || secondEvents[len(secondEvents)-1].Type != "Match.Unloaded" {
		t.Fatalf("explicit unload was not recorded: %+v, %v", secondEvents, err)
	}
}

func TestQuestionRevisionAdvancesOnlySelectedMatch(t *testing.T) {
	ctx := context.Background()
	service, closeStore := newService(t, ctx)
	defer closeStore()
	actor := game.Actor{UserID: 1}
	set, err := service.SaveQuestionSet(ctx, "", game.SaveQuestionSetInput{Name: "Đề chung", Data: validQuestions()}, actor)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := service.CreateMatch(ctx, matchInput("Trận 1", set.ID), actor)
	second, _ := service.CreateMatch(ctx, matchInput("Trận 2", set.ID), actor)
	if _, err = service.SelectMatch(ctx, first.ID, actor); err != nil {
		t.Fatal(err)
	}
	updated, err := service.SaveQuestionSet(ctx, set.ID, game.SaveQuestionSetInput{Name: set.Name, Data: validQuestions(), Version: set.Version}, actor)
	if err != nil {
		t.Fatal(err)
	}
	firstAfter, _ := service.GetMatch(ctx, first.ID)
	secondAfter, _ := service.GetMatch(ctx, second.ID)
	if firstAfter.QuestionSetRevisionID != updated.CurrentRevisionID {
		t.Fatal("selected match did not advance to the new revision")
	}
	if secondAfter.QuestionSetRevisionID == updated.CurrentRevisionID {
		t.Fatal("unselected match advanced unexpectedly")
	}
}
func TestLegacyQuestionDataIsNormalizedOnSave(t *testing.T) {
	ctx := context.Background()
	service, closeStore := newService(t, ctx)
	defer closeStore()
	legacy := json.RawMessage(`{"kd":{"chung":[{"content":"chung"}],"rieng":[[],[],[],[]]},"vcnv":{"obstacle":"Ô chữ","rows":[{},{},{},{}],"center_ques":"Câu trung tâm"},"tt":[{"content":"Tăng tốc"}],"vd":[{"20":[{"content":"A"}],"30":[{"content":"B"}]},{"20":[],"30":[]},{"20":[],"30":[]},{"20":[],"30":[]}]}`)
	set, err := service.SaveQuestionSet(ctx, "", game.SaveQuestionSetInput{Name: "Legacy", Data: legacy}, game.Actor{UserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, revision, err := service.GetQuestionSet(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	var normalized map[string]json.RawMessage
	if err := json.Unmarshal(revision.Data, &normalized); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"kd_chung", "kd_rieng", "vcnv", "tt", "vd"} {
		if len(normalized[key]) == 0 {
			t.Fatalf("normalized data is missing %s: %s", key, revision.Data)
		}
	}
	var tt []map[string]any
	if err := json.Unmarshal(normalized["tt"], &tt); err != nil || tt[0]["type"] != "text" {
		t.Fatalf("legacy tt question was not normalized: %s", normalized["tt"])
	}
}

func TestCurrentVCNVOnlyRevealsSolvedRowsAndObstacle(t *testing.T) {
	ctx := context.Background()
	service, closeStore := newService(t, ctx)
	defer closeStore()
	actor := game.Actor{UserID: 1}
	questions := json.RawMessage(`{"kd_chung":[{"content":"Câu hỏi","answer":"Đáp án","type":"text"}],"kd_rieng":[[],[],[],[]],"vcnv":{"obstacle":"Chướng ngại vật","obstacle_asset_id":"asset","rows":[{"keyword":"ĐIỆN - BIÊN"},{"keyword":"MẬT- MÃ"},{"keyword":"BÍ MẬT"},{"keyword":"ngựa"}],"center":{}},"tt":[],"vd":[]}`)
	set, err := service.SaveQuestionSet(ctx, "", game.SaveQuestionSetInput{Name: "VCNV lengths", Data: questions}, actor)
	if err != nil {
		t.Fatal(err)
	}
	match, err := service.CreateMatch(ctx, matchInput("VCNV", set.ID), actor)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.SelectMatch(ctx, match.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	state, err = service.Transition(ctx, game.StatusPlaying, command(state), actor)
	if err != nil {
		t.Fatal(err)
	}
	display, err := service.CurrentVCNV(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{8, 5, 5, 4}; !reflect.DeepEqual(display.RowLengths, want) {
		t.Fatalf("unexpected VCNV row lengths: got %v, want %v", display.RowLengths, want)
	}
	if display.Obstacle != "" || display.ObstacleLength != 13 {
		t.Fatalf("unsolved obstacle was exposed or its public length was wrong: %+v", display)
	}
	if want := []string{"", "", "", ""}; !reflect.DeepEqual(display.RowAnswers, want) {
		t.Fatalf("unsolved row answers were exposed: %q", display.RowAnswers)
	}
	encoded, err := json.Marshal(display)
	if err != nil {
		t.Fatal(err)
	}
	for _, answer := range []string{"ĐIỆN", "MẬT", "BÍ MẬT", "ngựa", "Chướng ngại vật"} {
		if strings.Contains(string(encoded), answer) {
			t.Fatalf("public VCNV response leaked hidden answer %q: %s", answer, encoded)
		}
	}
	state, err = service.SetVCNVRowState(ctx, 1, "c", command(state), actor)
	if err != nil {
		t.Fatal(err)
	}
	display, err = service.CurrentVCNV(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if display.RowAnswers[0] != "ĐIỆNBIÊN" || display.RowAnswers[1] != "" || display.RowAnswers[2] != "" || display.RowAnswers[3] != "" {
		t.Fatalf("only the solved row should be displayed: %q", display.RowAnswers)
	}
	state, err = service.SetVCNVSolved(ctx, true, command(state), actor)
	if err != nil {
		t.Fatal(err)
	}
	display, err = service.CurrentVCNV(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if display.Obstacle != "Chướng ngại vật" {
		t.Fatalf("solved obstacle should be visible: %q", display.Obstacle)
	}
}

func TestCompletionFinalizesResultAndRejectsRuntimeScoreChanges(t *testing.T) {
	ctx := context.Background()
	service, closeStore := newService(t, ctx)
	defer closeStore()
	actor := game.Actor{UserID: 1}
	set, err := service.SaveQuestionSet(ctx, "", game.SaveQuestionSetInput{Name: "Đề chung", Data: validQuestions()}, actor)
	if err != nil {
		t.Fatal(err)
	}
	match, err := service.CreateMatch(ctx, matchInput("Chung kết", set.ID), actor)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.SelectMatch(ctx, match.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	state, err = service.Transition(ctx, game.StatusPlaying, command(state), actor)
	if err != nil {
		t.Fatal(err)
	}
	state, err = service.AdjustScore(ctx, 1, 30, "đúng câu hỏi", command(state), actor)
	if err != nil {
		t.Fatal(err)
	}
	state, err = service.Transition(ctx, game.StatusPaused, command(state), actor)
	if err != nil {
		t.Fatal(err)
	}
	state, err = service.Transition(ctx, game.StatusCompleted, command(state), actor)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := service.GetMatch(ctx, match.ID)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Contestants []game.ContestantState `json:"contestants"`
	}
	if err := json.Unmarshal(completed.Result, &result); err != nil {
		t.Fatalf("final result is not valid JSON: %v", err)
	}
	if len(result.Contestants) != 4 || result.Contestants[0].Score != 30 {
		t.Fatalf("unexpected final result: %+v", result)
	}
	if _, err := service.AdjustScore(ctx, 1, 10, "late change", command(state), actor); err == nil {
		t.Fatal("expected a completed match score change to fail")
	}
}

func TestContestantAnswersAreScopedByPrincipal(t *testing.T) {
	ctx := context.Background()
	service, closeStore := newService(t, ctx)
	defer closeStore()
	actor := game.Actor{UserID: 1}
	var questions map[string]json.RawMessage
	if err := json.Unmarshal(validQuestions(), &questions); err != nil {
		t.Fatal(err)
	}
	questions["tt"] = json.RawMessage(`[{"content":"Câu hỏi gốc","answer":"đáp án gốc","type":"text","time":17}]`)
	data, err := json.Marshal(questions)
	if err != nil {
		t.Fatal(err)
	}
	set, _ := service.SaveQuestionSet(ctx, "", game.SaveQuestionSetInput{Name: "Đề chung", Data: data}, actor)
	match, _ := service.CreateMatch(ctx, matchInput("Bán kết", set.ID), actor)
	state, _ := service.SelectMatch(ctx, match.ID, actor)
	state, _ = service.Transition(ctx, game.StatusPlaying, command(state), actor)
	controllerState := service.Controller(ctx)
	controllerState.Phase = controller.PhaseTangToc
	controllerState.Screen = controller.ScreenQues
	state, err = service.UpdateController(ctx, controllerState, command(state), actor)
	if err != nil {
		t.Fatal(err)
	}
	controllerState = service.Controller(ctx)
	controllerState.SelectedQuestionID = "tt.0"
	controllerState.IsShowingQuestion = true
	state, err = service.UpdateController(ctx, controllerState, command(state), actor)
	if err != nil {
		t.Fatal(err)
	}
	controllerState = service.Controller(ctx)
	controllerState.IsShowingQuestion = true
	state, err = service.UpdateController(ctx, controllerState, command(state), actor)
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentQuestion == nil || state.CurrentQuestion.Answer != "đáp án gốc" {
		t.Fatalf("admin question snapshot was not loaded: %+v", state.CurrentQuestion)
	}
	publicBeforeAnswer := service.Current(opsocket.Principal{})
	if publicBeforeAnswer.CurrentQuestion == nil || publicBeforeAnswer.CurrentQuestion.Answer != "" {
		t.Fatalf("public question leaked its answer: %+v", publicBeforeAnswer.CurrentQuestion)
	}
	changedQuestions := json.RawMessage(`{"kd_chung":[{"content":"Câu hỏi đã sửa","answer":"đáp án mới","type":"text"}],"kd_rieng":[[],[],[],[]],"vcnv":{"obstacle":"A","obstacle_asset_id":"asset","rows":[{},{},{},{}],"center":{}},"tt":[{"content":"Tăng tốc đã sửa","answer":"đáp án mới","type":"text","time":49}],"vd":[]}`)
	if _, err = service.SaveQuestionSet(ctx, set.ID, game.SaveQuestionSetInput{Name: set.Name, Data: changedQuestions, Version: set.Version}, actor); err != nil {
		t.Fatal(err)
	}
	state = service.Current(opsocket.Principal{Roles: []string{string(auth.RoleAdmin)}})
	if state.CurrentQuestion == nil || state.CurrentQuestion.Content != "Câu hỏi gốc" {
		t.Fatalf("an already selected question changed after a revision: %+v", state.CurrentQuestion)
	}
	state, err = service.StartTimer(ctx, game.TimerStart{Command: command(state)}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if state.Timer.DurationMS != 17_000 {
		t.Fatalf("timer used the latest revision instead of the selected snapshot: %+v", state.Timer)
	}
	defer service.ResetTimer(ctx, game.Command{}, actor)
	state, err = service.SubmitAnswer(ctx, 1, "  đáp án A  ", command(state), game.Actor{UserID: 2})
	if err != nil {
		t.Fatal(err)
	}
	if state.Contestants[0].Answer != "đáp án A" || state.Contestants[0].Locked {
		t.Fatalf("answer was not recorded, or sending it locked further answers: %+v", state.Contestants[0])
	}
	public := service.Current(opsocket.Principal{})
	if public.Contestants[0].Answer != "" {
		t.Fatal("public state leaked a contestant answer")
	}
	contestant := service.Current(opsocket.Principal{Subject: "2", Claims: map[string]string{"slot": "1"}})
	if contestant.Contestants[0].Answer != "đáp án A" {
		t.Fatal("contestant could not see their own answer")
	}
	if contestant.Contestants[1].Answer != "" {
		t.Fatal("contestant state leaked another slot's answer")
	}
}

func TestCanonicalRoundMutationsPersistAndFilterRuntime(t *testing.T) {
	ctx := context.Background()
	service, closeStore := newService(t, ctx)
	defer closeStore()
	actor := game.Actor{UserID: 1}
	set, err := service.SaveQuestionSet(ctx, "", game.SaveQuestionSetInput{Name: "Đề bốn vòng", Data: validQuestions()}, actor)
	if err != nil {
		t.Fatal(err)
	}
	match, err := service.CreateMatch(ctx, matchInput("Bốn vòng", set.ID), actor)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.SelectMatch(ctx, match.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	state, err = service.Transition(ctx, game.StatusPlaying, command(state), actor)
	if err != nil {
		t.Fatal(err)
	}

	admin := opsocket.Principal{Roles: []string{string(auth.RoleAdmin)}}
	mutate := func(event string, apply func(game.Command) (game.RuntimeState, error)) {
		t.Helper()
		events, err := service.ListEvents(ctx, match.ID)
		if err != nil {
			t.Fatal(err)
		}
		beforeVersion := state.Version
		state, err = apply(command(state))
		if err != nil {
			t.Fatal(err)
		}
		if state.Version != beforeVersion+1 {
			t.Fatalf("%s advanced version from %d to %d, want one step", event, beforeVersion, state.Version)
		}
		eventsAfter, err := service.ListEvents(ctx, match.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(eventsAfter) != len(events)+1 || eventsAfter[len(eventsAfter)-1].Type != event {
			t.Fatalf("%s did not persist exactly one matching event: before=%d after=%+v", event, len(events), eventsAfter)
		}
		if current := service.Current(admin); current.Version != state.Version {
			t.Fatalf("%s did not update admin runtime: %+v", event, current)
		}
	}
	update := func(phase controller.Phase, screen controller.Screen, configure func(*controller.State)) {
		t.Helper()
		next := service.Controller(ctx)
		next.Phase = phase
		next.Screen = screen
		if configure != nil {
			configure(&next)
		}
		mutate("Controller.Updated", func(cmd game.Command) (game.RuntimeState, error) {
			return service.UpdateController(ctx, next, cmd, actor)
		})
	}

	update(controller.PhaseKhoiDong, controller.ScreenStart, nil)
	update(controller.PhaseKhoiDong, controller.ScreenRule, nil)
	update(controller.PhaseKhoiDong, controller.ScreenIntro, nil)
	update(controller.PhaseKhoiDong, controller.ScreenMainKD, nil)
	update(controller.PhaseKhoiDong, controller.ScreenQuesChung, func(next *controller.State) {
		next.SelectedQuestionID = "kd_chung.0"
		next.IsShowingQuestion = true
	})
	update(controller.PhaseKhoiDong, controller.ScreenQuesChung, func(next *controller.State) {
		next.IsShowingQuestion = true
	})
	if current := service.Current(admin); current.CurrentQuestion == nil || current.CurrentQuestion.Answer != "đáp án gốc" {
		t.Fatalf("admin did not receive selected Khởi động question: %+v", current.CurrentQuestion)
	}
	if current := service.Current(opsocket.Principal{}); current.Controller.Phase != controller.PhaseKhoiDong || current.Controller.Screen != controller.ScreenQuesChung || current.CurrentQuestion == nil || current.CurrentQuestion.Answer != "" {
		t.Fatalf("public Khởi động runtime was not correctly filtered: %+v", current)
	}
	update(controller.PhaseKhoiDong, controller.ScreenEnd, nil)

	update(controller.PhaseVCNV, controller.ScreenStart, nil)
	update(controller.PhaseVCNV, controller.ScreenRule, nil)
	update(controller.PhaseVCNV, controller.ScreenIntro, nil)
	update(controller.PhaseVCNV, controller.ScreenMainVCNV, nil)
	mutate("Controller.VCNVStarted", func(cmd game.Command) (game.RuntimeState, error) {
		return service.SetVCNVStarted(ctx, true, cmd, actor)
	})
	mutate("Controller.VCNVRowStateChanged", func(cmd game.Command) (game.RuntimeState, error) {
		return service.SetVCNVRowState(ctx, 2, "c", cmd, actor)
	})
	update(controller.PhaseVCNV, controller.ScreenImageVCNV, nil)
	mutate("Controller.VCNVPanelChanged", func(cmd game.Command) (game.RuntimeState, error) {
		return service.SetVCNVPanelOpen(ctx, 3, true, cmd, actor)
	})
	update(controller.PhaseVCNV, controller.ScreenQues, nil)
	mutate("Controller.VCNVSolved", func(cmd game.Command) (game.RuntimeState, error) {
		return service.SetVCNVSolved(ctx, true, cmd, actor)
	})
	if current := service.Current(admin); !current.Controller.VCNVState.HasStarted || !current.Controller.VCNVState.HasSolved || current.Controller.VCNVState.RowStates[1] != "c" || !current.Controller.VCNVState.ImageOpenState[3] {
		t.Fatalf("VCNV mutations were not retained: %+v", current.Controller.VCNVState)
	}
	update(controller.PhaseVCNV, controller.ScreenEnd, nil)

	update(controller.PhaseTangToc, controller.ScreenStart, nil)
	update(controller.PhaseTangToc, controller.ScreenRule, nil)
	update(controller.PhaseTangToc, controller.ScreenIntro, nil)
	update(controller.PhaseTangToc, controller.ScreenQues, nil)
	update(controller.PhaseTangToc, controller.ScreenSolve, nil)
	update(controller.PhaseTangToc, controller.ScreenEnd, nil)

	update(controller.PhaseVeDich, controller.ScreenStart, nil)
	update(controller.PhaseVeDich, controller.ScreenRule, nil)
	update(controller.PhaseVeDich, controller.ScreenIntro, nil)
	update(controller.PhaseVeDich, controller.ScreenMainVD, nil)
	update(controller.PhaseVeDich, controller.ScreenPreQuesTS1, func(next *controller.State) {
		next.SelectedContestant = 1
	})
	mutate("Controller.VeDichQuestionSetSelected", func(cmd game.Command) (game.RuntimeState, error) {
		return service.SelectVeDichQuestionSet(ctx, 1, []string{"20_1", "20_2", "30_3"}, cmd, actor)
	})
	update(controller.PhaseVeDich, controller.ScreenQuesTS1, nil)
	mutate("Controller.VeDichStarChanged", func(cmd game.Command) (game.RuntimeState, error) {
		return service.SetVeDichStar(ctx, 1, 2, cmd, actor)
	})
	if current := service.Current(admin); current.Controller.SelectedContestant != 1 || !reflect.DeepEqual(current.Controller.VeDichContestantsState[0].SelectedQuestionSet, []string{"20_1", "20_2", "30_3"}) || current.Controller.VeDichContestantsState[0].StarredOn != 2 {
		t.Fatalf("Về đích mutations were not retained: %+v", current.Controller)
	}
	update(controller.PhaseVeDich, controller.ScreenEndTS, nil)
	update(controller.PhaseVeDich, controller.ScreenEnd, nil)
}

func TestInvalidRoundMutationsLeaveRuntimeAndEventsUnchanged(t *testing.T) {
	ctx := context.Background()
	service, closeStore := newService(t, ctx)
	defer closeStore()
	actor := game.Actor{UserID: 1}
	set, err := service.SaveQuestionSet(ctx, "", game.SaveQuestionSetInput{Name: "Đề kiểm tra", Data: validQuestions()}, actor)
	if err != nil {
		t.Fatal(err)
	}
	match, err := service.CreateMatch(ctx, matchInput("Kiểm tra", set.ID), actor)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.SelectMatch(ctx, match.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	state, err = service.Transition(ctx, game.StatusPlaying, command(state), actor)
	if err != nil {
		t.Fatal(err)
	}
	admin := opsocket.Principal{Roles: []string{string(auth.RoleAdmin)}}
	assertRejected := func(field string, apply func(game.Command) (game.RuntimeState, error)) {
		t.Helper()
		before := service.Current(admin)
		events, err := service.ListEvents(ctx, match.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := apply(command(before)); err == nil {
			t.Fatalf("expected validation error for %s", field)
		} else if socketErr, ok := err.(*opsocket.Error); !ok || socketErr.Code != "VALIDATION" || socketErr.Details["field"] != field {
			t.Fatalf("unexpected validation error for %s: %#v", field, err)
		}
		if after := service.Current(admin); !reflect.DeepEqual(after, before) {
			t.Fatalf("rejected %s mutation changed runtime: before=%+v after=%+v", field, before, after)
		}
		eventsAfter, err := service.ListEvents(ctx, match.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(eventsAfter) != len(events) {
			t.Fatalf("rejected %s mutation persisted an event: before=%d after=%d", field, len(events), len(eventsAfter))
		}
	}

	assertRejected("row", func(cmd game.Command) (game.RuntimeState, error) {
		return service.SetVCNVRowState(ctx, 5, "c", cmd, actor)
	})
	assertRejected("state", func(cmd game.Command) (game.RuntimeState, error) {
		return service.SetVCNVRowState(ctx, 1, "invalid", cmd, actor)
	})
	assertRejected("index", func(cmd game.Command) (game.RuntimeState, error) {
		return service.SetVCNVPanelOpen(ctx, 5, true, cmd, actor)
	})
	assertRejected("slot", func(cmd game.Command) (game.RuntimeState, error) {
		return service.SelectVeDichQuestionSet(ctx, 5, []string{"20_1", "20_2", "20_3"}, cmd, actor)
	})
	assertRejected("question_ids[1]", func(cmd game.Command) (game.RuntimeState, error) {
		return service.SelectVeDichQuestionSet(ctx, 1, []string{"20_1", "bad", "20_3"}, cmd, actor)
	})
	assertRejected("question", func(cmd game.Command) (game.RuntimeState, error) {
		return service.SetVeDichStar(ctx, 1, 4, cmd, actor)
	})
}

func newService(t *testing.T, ctx context.Context) (*game.Service, func()) {
	t.Helper()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	authService := auth.NewService(database)
	if _, err = authService.EnsureFixedContestants(ctx); err != nil {
		t.Fatal(err)
	}
	fixed, err := authService.ListFixedContestants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	service, err := game.NewService(ctx, database, fixed, nil)
	if err != nil {
		t.Fatal(err)
	}
	return service, func() { _ = database.Close() }
}

func matchInput(name, setID string) game.CreateMatchInput {
	return game.CreateMatchInput{Name: name, Season: 9, Stage: "Bán kết", QuestionSetID: setID, Contestants: []game.Contestant{{Slot: 1, Name: "A", Class: "10A"}, {Slot: 2, Name: "B", Class: "10B"}, {Slot: 3, Name: "C", Class: "10C"}, {Slot: 4, Name: "D", Class: "10D"}}}
}
func command(state game.RuntimeState) game.Command {
	return game.Command{MatchID: state.MatchID, ExpectedEpoch: state.Epoch, ExpectedVersion: state.Version, ExpectedInputEpoch: state.InputEpoch, ExpectedBellEpoch: state.BellEpoch, CommandID: fmt.Sprintf("%s-%d", state.MatchID, state.Version)}
}
func validQuestions() json.RawMessage {
	return json.RawMessage(`{"kd_chung":[{"content":"Câu hỏi gốc","answer":"đáp án gốc","type":"text"}],"kd_rieng":[[],[],[],[]],"vcnv":{"obstacle":"A","obstacle_asset_id":"asset","rows":[{},{},{},{}],"center":{}},"tt":[],"vd":[]}`)
}

func TestCancelledTimerCannotFinishReplacementTimer(t *testing.T) {
	ctx := context.Background()
	service, state := timerPolicyMatch(t)
	state = selectTimerQuestion(t, service, state, controller.PhaseTangToc, controller.ScreenQues, "tt.0")
	must := mustRuntime(t)
	state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor))
	if state.Timer.DurationMS != 1_000 {
		t.Fatalf("first question did not get its configured duration: %+v", state.Timer)
	}
	state = must(service.ResetTimer(ctx, command(state), parityActor))
	state = selectTimerQuestion(t, service, state, controller.PhaseTangToc, controller.ScreenQues, "tt.1")
	state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor))
	time.Sleep(1_100 * time.Millisecond)
	current := service.Current(parityAdmin)
	if !current.Timer.IsRunning || current.Timer.DurationMS != 4_000 || current.Timer.RemainingMS <= 0 {
		t.Fatalf("cancelled timer cleared replacement timer: %+v", current.Timer)
	}
	events, err := service.ListEvents(ctx, state.MatchID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == "Timer.Finished" {
			t.Fatal("cancelled timer persisted a spurious finish event")
		}
	}
}
func TestPausedTimerResumesWithRemainingDuration(t *testing.T) {
	ctx := context.Background()
	service, state := timerPolicyMatch(t)
	state = selectTimerQuestion(t, service, state, controller.PhaseTangToc, controller.ScreenQues, "tt.1")
	must := mustRuntime(t)
	state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor))
	time.Sleep(35 * time.Millisecond)
	paused := must(service.PauseTimer(ctx, command(state), parityActor))
	if !paused.Timer.IsPaused || paused.Timer.RemainingMS >= 4_000 || paused.Timer.RemainingMS <= 0 || paused.Timer.DurationMS != 4_000 {
		t.Fatalf("timer did not retain remaining duration: %+v", paused.Timer)
	}
	resumed := must(service.ResumeTimer(ctx, command(paused), parityActor))
	if !resumed.Timer.IsRunning || resumed.Timer.IsPaused || resumed.Timer.DurationMS != 4_000 ||
		resumed.Timer.RemainingMS != paused.Timer.RemainingMS || resumed.Timer.ElapsedMS != paused.Timer.ElapsedMS {
		t.Fatalf("timer resume changed remaining duration: %+v", resumed.Timer)
	}
}

func TestTimerCompletionAfterUnloadCannotResurrectRuntime(t *testing.T) {
	ctx := context.Background()
	service, state := timerPolicyMatch(t)
	state = selectTimerQuestion(t, service, state, controller.PhaseTangToc, controller.ScreenQues, "tt.0")
	must := mustRuntime(t)
	state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor))
	state = must(service.PauseTimer(ctx, command(state), parityActor))
	state = must(service.Transition(ctx, game.StatusPaused, command(state), parityActor))
	_ = must(service.UnloadMatch(ctx, command(state), parityActor))
	time.Sleep(1_100 * time.Millisecond)
	if current := service.Current(opsocket.Principal{}); current.MatchID != "" || current.Timer.IsRunning {
		t.Fatalf("stale timer resurrected unloaded runtime: %+v", current)
	}
}

func TestArchivedQuestionSetCannotBeAssignedToMatch(t *testing.T) {
	ctx := context.Background()
	service, closeStore := newService(t, ctx)
	defer closeStore()
	actor := game.Actor{UserID: 1}
	set, err := service.SaveQuestionSet(ctx, "", game.SaveQuestionSetInput{Name: "Đề lưu trữ", Data: validQuestions()}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveQuestionSet(ctx, set.ID, set.Version, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateMatch(ctx, matchInput("Không hợp lệ", set.ID), actor); err == nil {
		t.Fatal("expected archived question set assignment to fail")
	}
}
