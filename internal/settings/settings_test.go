package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCreatesDefaultsAndPersistsUpdates(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "settings.yaml")
	service, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Get(); got != Defaults() {
		t.Fatalf("unexpected defaults: %+v", got)
	}

	want := Values{
		EventName: "  Demo event  ",
		HostName:  "  Demo host  ",
	}
	updated, err := service.Update(want)
	if err != nil {
		t.Fatal(err)
	}
	if updated.EventName != "Demo event" || updated.HostName != "Demo host" {
		t.Fatalf("values were not normalized: %+v", updated)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Get(); got != updated {
		t.Fatalf("persisted settings mismatch: got %+v want %+v", got, updated)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "event_name: Demo event") {
		t.Fatalf("settings were not written as readable YAML:\n%s", contents)
	}
}

func TestUpdateRejectsInvalidValuesWithoutChangingMemoryOrFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "settings.yaml")
	service, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeFile, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	invalid := Defaults()
	invalid.EventName = ""
	if _, err := service.Update(invalid); err == nil {
		t.Fatal("expected validation error")
	}
	if got := service.Get(); got != Defaults() {
		t.Fatalf("invalid update changed memory: %+v", got)
	}
	afterFile, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterFile) != string(beforeFile) {
		t.Fatal("invalid update changed the settings file")
	}
}

func TestOpenRejectsUnknownYAMLFields(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := os.WriteFile(path, []byte("event_name: 4T\nunknown: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "field unknown") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestScoringDefaultsMatchOlympiaRules(t *testing.T) {
	t.Parallel()
	scoring := Defaults().Scoring
	if scoring.KhoiDong.CommonWrong != -5 || !scoring.KhoiDong.CommonFloorZero || scoring.KhoiDong.IndividualCorrect != 10 {
		t.Fatalf("unexpected Khởi động defaults: %+v", scoring.KhoiDong)
	}
	if scoring.VCNV.ObstacleByRows != [4]int{60, 50, 40, 30} || scoring.VCNV.ObstacleAfterCenter != 20 {
		t.Fatalf("unexpected VCNV defaults: %+v", scoring.VCNV)
	}
	if scoring.TangToc.Ranks != [4]int{40, 30, 20, 10} {
		t.Fatalf("unexpected Tăng tốc defaults: %+v", scoring.TangToc)
	}
	if scoring.VeDich.Tiers != [2]int{20, 30} || scoring.VeDich.StarCorrectPercent != 200 || scoring.VeDich.StealWrongPercent != -50 || scoring.VeDich.StolenPercent != -100 || scoring.VeDich.StolenStarPercent != 0 {
		t.Fatalf("unexpected Về đích defaults: %+v", scoring.VeDich)
	}
}

func TestLegacySettingsWithoutScoringGetDefaults(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := os.WriteFile(path, []byte("event_name: 4T\nhost_name: \"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Get().Scoring; got != DefaultScoring() {
		t.Fatalf("legacy settings did not receive default scoring: %+v", got)
	}
}

func TestScoringUpdatesPersistAndPartialSectionsAreFilled(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "settings.yaml")
	service, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	values := service.Get()
	values.Scoring.VeDich.Tiers = [2]int{20, 40}
	values.Scoring.TangToc.Ranks = [4]int{50, 30, 20, 10}
	values.Scoring.Adjust = AdjustScoring{}
	updated, err := service.Update(values)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Scoring.Adjust != DefaultScoring().Adjust {
		t.Fatalf("missing nudge section was not filled: %+v", updated.Scoring.Adjust)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Get().Scoring
	if got.VeDich.Tiers != [2]int{20, 40} || got.TangToc.Ranks != [4]int{50, 30, 20, 10} {
		t.Fatalf("scoring was not persisted: %+v", got)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "obstacle_by_rows:") {
		t.Fatalf("scoring was not written as readable YAML:\n%s", contents)
	}
}

func TestScoringRejectsOutOfRangeValues(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*ScoringValues){
		"points":        func(s *ScoringValues) { s.KhoiDong.CommonCorrect = 1001 },
		"tier zero":     func(s *ScoringValues) { s.VeDich.Tiers = [2]int{0, 30} },
		"equal tiers":   func(s *ScoringValues) { s.VeDich.Tiers = [2]int{30, 30} },
		"percent":       func(s *ScoringValues) { s.VeDich.StealWrongPercent = -1001 },
		"nudge":         func(s *ScoringValues) { s.Adjust = AdjustScoring{Down: 0, Up: 10} },
		"obstacle rows": func(s *ScoringValues) { s.VCNV.ObstacleByRows[3] = -2000 },
	}
	for name, mutate := range cases {
		values := Defaults()
		mutate(&values.Scoring)
		if _, err := validate(values); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}
