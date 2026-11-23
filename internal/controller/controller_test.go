package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
)

func TestDefaultStateValidatesAndSerializesCollections(t *testing.T) {
	state := DefaultState()
	if err := ValidateState(state); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), ":null") {
		t.Fatalf("default controller state contains null collections: %s", encoded)
	}
}

func TestValidateStateRejectsInvalidScreenAndVCNVData(t *testing.T) {
	state := DefaultState()
	state.Phase = PhaseVCNV
	state.Screen = ScreenMainKD
	assertValidationField(t, ValidateState(state), "screen")

	state = DefaultState()
	state.VCNVState.RowStates[0] = "invalid"
	assertValidationField(t, ValidateState(state), "vcnv_state.row_states[0]")

	state = DefaultState()
	state.VCNVState.ImageOpenState = []bool{false}
	assertValidationField(t, ValidateState(state), "vcnv_state.image_open_state")
}

func TestValidateStateRejectsInvalidVeDichPackage(t *testing.T) {
	state := DefaultState()
	state.VeDichContestantsState[0].SelectedQuestionSet = []string{"10_1"}
	err := ValidateState(state)
	assertValidationField(t, err, "ve_dich_contestants_state[0].selected_question_set")
}

func assertValidationField(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation error for %s", field)
	}
	status, ok := err.(*opsocket.Error)
	if !ok || status.Code != "VALIDATION" || status.Details["field"] != field {
		t.Fatalf("unexpected validation error: %#v", err)
	}
}
