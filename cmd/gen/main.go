package main

import (
	"bytes"
	"fmt"
	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
	"github.com/onlyin32bit/4t/internal/settings"
	"github.com/onlyin32bit/4t/internal/sound"
	"github.com/onlyin32bit/4t/internal/web"
	"os"
	"path/filepath"
)

func main() {
	socket := opsocket.NewSocket()
	if err := web.RegisterSocketControllers(socket, nil, nil); err != nil {
		panic(err)
	}
	var output bytes.Buffer
	err := socket.WriteTypeScript(&output, opsocket.TypeScriptSchema{
		Events:  map[string]any{"Match.Updated": game.RuntimeState{}, "Match.PublicUpdated": game.RuntimeState{}, "Match.ContestantUpdated": game.RuntimeState{}, "Sound.Played": sound.Playback{}, "Sound.Stopped": sound.StopEvent{}, "Sound.CatalogUpdated": struct{}{}, "Sound.SettingsUpdated": settings.SoundValues{}, "Settings.Updated": struct{}{}},
		Aliases: map[string]any{"MatchCommand": game.Command{}, "MatchRuntimeState": game.RuntimeState{}, "MatchStatus": game.MatchStatus(""), "TimerState": game.TimerState{}, "RuntimeContestant": game.ContestantState{}, "SoundPlayback": sound.Playback{}, "SoundStatus": sound.Status{}, "Phase": controller.Phase(""), "Screen": controller.Screen("")},
		Enums:   map[string][]string{"MatchStatus": {"draft", "ready", "playing", "paused", "completed", "archived"}, "Phase": {"idle", "logo", "main", "score", "khoi_dong", "vcnv", "tang_toc", "ve_dich"}, "Screen": {"intro", "main", "score", "start", "rule", "main_kd", "test_bell", "ques_chung", "ques_ts1", "ques_ts2", "ques_ts3", "ques_ts4", "end_ts", "end", "main_vcnv", "image_vcnv", "ques", "answers_vcnv", "answers_tt", "solve", "main_vd", "pre_ques_ts1", "pre_ques_ts2", "pre_ques_ts3", "pre_ques_ts4"}},
	})
	if err != nil {
		panic(err)
	}
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	target := filepath.Join(root, "ui", "src", "lib", "socket", "opsocket.d.ts")
	if err = os.WriteFile(target, output.Bytes(), 0o644); err != nil {
		panic(err)
	}
	fmt.Println("generated", target)
}
