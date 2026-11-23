package settings

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

type Values struct {
	EventName string        `json:"event_name" yaml:"event_name"`
	HostName  string        `json:"host_name" yaml:"host_name"`
	Sound     SoundValues   `json:"sound" yaml:"sound"`
	Scoring   ScoringValues `json:"scoring" yaml:"scoring"`
}

// ScoringValues holds every point value the operator panel suggests. Saved
// scores are never recalculated when these change.
type ScoringValues struct {
	KhoiDong KhoiDongScoring `json:"khoi_dong" yaml:"khoi_dong"`
	VCNV     VCNVScoring     `json:"vcnv" yaml:"vcnv"`
	TangToc  TangTocScoring  `json:"tang_toc" yaml:"tang_toc"`
	VeDich   VeDichScoring   `json:"ve_dich" yaml:"ve_dich"`
	Adjust   AdjustScoring   `json:"adjust" yaml:"adjust"`
}

type KhoiDongScoring struct {
	IndividualCorrect int  `json:"individual_correct" yaml:"individual_correct"`
	IndividualWrong   int  `json:"individual_wrong" yaml:"individual_wrong"`
	CommonCorrect     int  `json:"common_correct" yaml:"common_correct"`
	CommonWrong       int  `json:"common_wrong" yaml:"common_wrong"`
	CommonFloorZero   bool `json:"common_floor_zero" yaml:"common_floor_zero"`
}

type VCNVScoring struct {
	RowCorrect          int    `json:"row_correct" yaml:"row_correct"`
	RowWrong            int    `json:"row_wrong" yaml:"row_wrong"`
	CenterCorrect       int    `json:"center_correct" yaml:"center_correct"`
	CenterWrong         int    `json:"center_wrong" yaml:"center_wrong"`
	ObstacleByRows      [4]int `json:"obstacle_by_rows" yaml:"obstacle_by_rows"`
	ObstacleAfterCenter int    `json:"obstacle_after_center" yaml:"obstacle_after_center"`
	ObstacleWrong       int    `json:"obstacle_wrong" yaml:"obstacle_wrong"`
}

type TangTocScoring struct {
	Ranks [4]int `json:"ranks" yaml:"ranks"`
	Wrong int    `json:"wrong" yaml:"wrong"`
}

// VeDichScoring percentages are applied to the question's tier value.
type VeDichScoring struct {
	Tiers               [2]int `json:"tiers" yaml:"tiers"`
	CorrectPercent      int    `json:"correct_percent" yaml:"correct_percent"`
	WrongPercent        int    `json:"wrong_percent" yaml:"wrong_percent"`
	StarCorrectPercent  int    `json:"star_correct_percent" yaml:"star_correct_percent"`
	StarWrongPercent    int    `json:"star_wrong_percent" yaml:"star_wrong_percent"`
	StealCorrectPercent int    `json:"steal_correct_percent" yaml:"steal_correct_percent"`
	StealWrongPercent   int    `json:"steal_wrong_percent" yaml:"steal_wrong_percent"`
	StolenPercent       int    `json:"stolen_percent" yaml:"stolen_percent"`
	StolenStarPercent   int    `json:"stolen_star_percent" yaml:"stolen_star_percent"`
}

type AdjustScoring struct {
	Down int `json:"down" yaml:"down"`
	Up   int `json:"up" yaml:"up"`
}

func DefaultScoring() ScoringValues {
	return ScoringValues{
		KhoiDong: KhoiDongScoring{IndividualCorrect: 10, CommonCorrect: 10, CommonWrong: -5, CommonFloorZero: true},
		VCNV:     VCNVScoring{RowCorrect: 10, CenterCorrect: 10, ObstacleByRows: [4]int{60, 50, 40, 30}, ObstacleAfterCenter: 20},
		TangToc:  TangTocScoring{Ranks: [4]int{40, 30, 20, 10}},
		VeDich: VeDichScoring{
			Tiers: [2]int{20, 30}, CorrectPercent: 100, StarCorrectPercent: 200, StarWrongPercent: -100,
			StealCorrectPercent: 100, StealWrongPercent: -50, StolenPercent: -100,
		},
		Adjust: AdjustScoring{Down: 5, Up: 10},
	}
}

type SoundValues struct {
	Enabled          bool    `json:"enabled" yaml:"enabled"`
	MasterVolume     float64 `json:"master_volume" yaml:"master_volume"`
	MusicVolume      float64 `json:"music_volume" yaml:"music_volume"`
	EffectsVolume    float64 `json:"effects_volume" yaml:"effects_volume"`
	TransitionFadeMS int     `json:"transition_fade_ms" yaml:"transition_fade_ms"`
	BellSoundID      string  `json:"bell_sound_id" yaml:"bell_sound_id"`
}

type diskValues struct {
	EventName        string         `yaml:"event_name"`
	HostName         string         `yaml:"host_name"`
	Season           *int           `yaml:"season,omitempty"`
	CompetitionStage *string        `yaml:"competition_stage,omitempty"`
	MatchNumber      *int           `yaml:"match_number,omitempty"`
	Sound            *SoundValues   `yaml:"sound,omitempty"`
	Scoring          *ScoringValues `yaml:"scoring,omitempty"`
}

func Defaults() Values {
	return Values{
		EventName: DefaultEventName,
		Sound:     SoundValues{Enabled: true, MasterVolume: 0.8, MusicVolume: 0.8, EffectsVolume: 1, TransitionFadeMS: 120, BellSoundID: "builtin.kd.bell"},
		Scoring:   DefaultScoring(),
	}
}

type Repository interface {
	LoadSettings(context.Context) (Values, error)
	SaveSettings(context.Context, Values) error
}
type AuditEntry struct {
	Action     string
	EntityType string
	EntityID   string
	Before     []byte
	After      []byte
	Reason     string
	ActorID    int64
	ClientID   string
}

type AtomicRepository interface {
	Repository
	SaveSettingsAndAudit(context.Context, Values, AuditEntry) error
}

var ErrAtomicRepository = errors.New("atomic settings repository unavailable")

func (s *Service) SupportsAtomicAudit() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.repo.(AtomicRepository)
	return ok
}

func (s *Service) UpdateWithAudit(ctx context.Context, values Values, audit AuditEntry) (Values, error) {
	values, err := validate(values)
	if err != nil {
		return Values{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.repo.(AtomicRepository)
	if !ok {
		return Values{}, ErrAtomicRepository
	}
	if err := repo.SaveSettingsAndAudit(ctx, values, audit); err != nil {
		return Values{}, err
	}
	s.values = values
	return values, nil
}

var ErrNotFound = errors.New("settings not found")

type Service struct {
	mu     sync.RWMutex
	path   string
	values Values
	repo   Repository
}

func Open(path string) (*Service, error) {
	values := Defaults()
	contents, err := os.ReadFile(path)
	missing := errors.Is(err, os.ErrNotExist)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read site settings: %w", err)
	}
	if err == nil {
		decoder := yaml.NewDecoder(bytes.NewReader(contents))
		decoder.KnownFields(true)
		legacy := diskValues{EventName: values.EventName, HostName: values.HostName}
		if err := decoder.Decode(&legacy); err != nil {
			return nil, fmt.Errorf("parse site settings: %w", err)
		}
		values = Values{EventName: legacy.EventName, HostName: legacy.HostName, Sound: values.Sound, Scoring: values.Scoring}
		if legacy.Sound != nil {
			values.Sound = *legacy.Sound
		}
		if legacy.Scoring != nil {
			values.Scoring = *legacy.Scoring
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			if err == nil {
				return nil, errors.New("parse site settings: multiple YAML documents are not supported")
			}
			return nil, fmt.Errorf("parse site settings: %w", err)
		}
	}

	values, err = validate(values)
	if err != nil {
		return nil, fmt.Errorf("validate site settings: %w", err)
	}

	service := &Service{path: path, values: values}
	if missing {
		if err := service.write(values); err != nil {
			return nil, err
		}
	}
	return service, nil
}
func OpenWithRepository(ctx context.Context, path string, repo Repository) (*Service, error) {
	service, err := Open(path)
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return service, nil
	}
	service.repo = repo
	values, err := repo.LoadSettings(ctx)
	switch {
	case err == nil:
		values, err = validate(values)
		if err != nil {
			return nil, fmt.Errorf("validate stored site settings: %w", err)
		}
		service.values = values
	case errors.Is(err, ErrNotFound):
		if err := repo.SaveSettings(ctx, service.values); err != nil {
			return nil, fmt.Errorf("store site settings: %w", err)
		}
	default:
		return nil, fmt.Errorf("load stored site settings: %w", err)
	}
	return service, nil
}

func (s *Service) Get() Values {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.values
}

func (s *Service) Update(values Values) (Values, error) {
	values, err := validate(values)
	if err != nil {
		return Values{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.repo != nil {
		if err := s.repo.SaveSettings(context.Background(), values); err != nil {
			return Values{}, err
		}
	} else if err := s.write(values); err != nil {
		return Values{}, err
	}
	s.values = values
	return values, nil
}

func validate(values Values) (Values, error) {
	values.EventName = strings.TrimSpace(values.EventName)
	values.HostName = strings.TrimSpace(values.HostName)
	if strings.TrimSpace(values.Sound.BellSoundID) == "" {
		values.Sound = Defaults().Sound
	}
	if values.EventName == "" {
		return Values{}, errors.New("event name is required")
	}
	if utf8.RuneCountInString(values.EventName) > 120 {
		return Values{}, errors.New("event name must not exceed 120 characters")
	}
	if utf8.RuneCountInString(values.HostName) > 120 {
		return Values{}, errors.New("host name must not exceed 120 characters")
	}
	if values.Sound.MasterVolume < 0 || values.Sound.MasterVolume > 1 || values.Sound.MusicVolume < 0 || values.Sound.MusicVolume > 1 || values.Sound.EffectsVolume < 0 || values.Sound.EffectsVolume > 1 {
		return Values{}, errors.New("sound volumes must be between 0 and 1")
	}
	if values.Sound.TransitionFadeMS < 0 || values.Sound.TransitionFadeMS > 10000 {
		return Values{}, errors.New("sound transition fade must be between 0 and 10000 milliseconds")
	}
	values.Sound.BellSoundID = strings.TrimSpace(values.Sound.BellSoundID)
	if values.Sound.BellSoundID == "" {
		return Values{}, errors.New("bell sound is required")
	}
	scoring, err := validateScoring(values.Scoring)
	if err != nil {
		return Values{}, err
	}
	values.Scoring = scoring
	return values, nil
}

const maxPoints = 1000

// validateScoring fills sections missing from older stored settings with
// defaults, then bounds every value so a typo cannot produce absurd scores.
func validateScoring(scoring ScoringValues) (ScoringValues, error) {
	defaults := DefaultScoring()
	if scoring.KhoiDong == (KhoiDongScoring{}) {
		scoring.KhoiDong = defaults.KhoiDong
	}
	if scoring.VCNV == (VCNVScoring{}) {
		scoring.VCNV = defaults.VCNV
	}
	if scoring.TangToc == (TangTocScoring{}) {
		scoring.TangToc = defaults.TangToc
	}
	if scoring.VeDich == (VeDichScoring{}) {
		scoring.VeDich = defaults.VeDich
	}
	if scoring.Adjust == (AdjustScoring{}) {
		scoring.Adjust = defaults.Adjust
	}
	points := []int{
		scoring.KhoiDong.IndividualCorrect, scoring.KhoiDong.IndividualWrong, scoring.KhoiDong.CommonCorrect, scoring.KhoiDong.CommonWrong,
		scoring.VCNV.RowCorrect, scoring.VCNV.RowWrong, scoring.VCNV.CenterCorrect, scoring.VCNV.CenterWrong,
		scoring.VCNV.ObstacleAfterCenter, scoring.VCNV.ObstacleWrong, scoring.TangToc.Wrong,
	}
	points = append(points, scoring.VCNV.ObstacleByRows[:]...)
	points = append(points, scoring.TangToc.Ranks[:]...)
	for _, value := range points {
		if value < -maxPoints || value > maxPoints {
			return ScoringValues{}, fmt.Errorf("point values must be between -%d and %d", maxPoints, maxPoints)
		}
	}
	for _, tier := range scoring.VeDich.Tiers {
		if tier < 1 || tier > maxPoints {
			return ScoringValues{}, fmt.Errorf("finish-round tiers must be between 1 and %d", maxPoints)
		}
	}
	if scoring.VeDich.Tiers[0] == scoring.VeDich.Tiers[1] {
		return ScoringValues{}, errors.New("finish-round tiers must be different")
	}
	for _, percent := range []int{
		scoring.VeDich.CorrectPercent, scoring.VeDich.WrongPercent, scoring.VeDich.StarCorrectPercent, scoring.VeDich.StarWrongPercent,
		scoring.VeDich.StealCorrectPercent, scoring.VeDich.StealWrongPercent, scoring.VeDich.StolenPercent, scoring.VeDich.StolenStarPercent,
	} {
		if percent < -1000 || percent > 1000 {
			return ScoringValues{}, errors.New("finish-round percentages must be between -1000 and 1000")
		}
	}
	if scoring.Adjust.Down < 1 || scoring.Adjust.Down > maxPoints || scoring.Adjust.Up < 1 || scoring.Adjust.Up > maxPoints {
		return ScoringValues{}, fmt.Errorf("score nudges must be between 1 and %d", maxPoints)
	}
	return scoring, nil
}

func (s *Service) write(values Values) error {
	contents, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("encode site settings: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create site settings directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".settings-*.yaml")
	if err != nil {
		return fmt.Errorf("create temporary site settings: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("protect temporary site settings: %w", err)
	}
	if _, err := temp.Write(contents); err != nil {
		temp.Close()
		return fmt.Errorf("write temporary site settings: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync temporary site settings: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary site settings: %w", err)
	}
	if err := os.Rename(tempName, s.path); err != nil {
		return fmt.Errorf("replace site settings: %w", err)
	}
	return nil
}
