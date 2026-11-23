package web

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
)

func TestDisplayParitySocketSurface(t *testing.T) {
	handler := &controllerSocket{}
	if _, err := handler.ClearBells(&opsocket.Ctx{}, transitionRequest{}); err == nil {
		t.Fatal("ClearBells allowed a caller without the admin role")
	} else {
		var operationError *opsocket.Error
		if !errors.As(err, &operationError) || operationError.Code != "FORBIDDEN" {
			t.Fatalf("ClearBells rejection = %v", err)
		}
	}
	if _, err := handler.ClearAnswers(&opsocket.Ctx{}, transitionRequest{}); err == nil {
		t.Fatal("ClearAnswers allowed a caller without the admin role")
	} else {
		var operationError *opsocket.Error
		if !errors.As(err, &operationError) || operationError.Code != "FORBIDDEN" {
			t.Fatalf("ClearAnswers rejection = %v", err)
		}
	}
	socket := opsocket.NewSocket()
	if err := RegisterSocketControllers(socket, nil, nil); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := socket.WriteTypeScript(&output, opsocket.TypeScriptSchema{Aliases: map[string]any{"TimerState": game.TimerState{}}}); err != nil {
		t.Fatal(err)
	}
	generated := output.String()
	for _, want := range []string{"Controller.ClearBells", "Controller.ClearAnswers", "obstacle?: boolean", "duration_ms?: number", "started_at: number", "position_ms: number"} {
		if !strings.Contains(generated, want) {
			t.Fatalf("generated socket types are missing %q", want)
		}
	}
}
