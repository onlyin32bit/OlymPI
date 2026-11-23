package game

import (
	"encoding/json"
	"time"

	"github.com/onlyin32bit/4t/internal/controller"
)

type MatchStatus string

const (
	StatusDraft     MatchStatus = "draft"
	StatusReady     MatchStatus = "ready"
	StatusPlaying   MatchStatus = "playing"
	StatusPaused    MatchStatus = "paused"
	StatusCompleted MatchStatus = "completed"
	StatusArchived  MatchStatus = "archived"
)

type Contestant struct {
	Slot   int    `json:"slot"`
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Class  string `json:"class"`
}

type ContestantState struct {
	Slot       int    `json:"slot"`
	Name       string `json:"name"`
	Class      string `json:"class"`
	Score      int    `json:"score"`
	Answer     string `json:"answer,omitempty"`
	TimeMS     int64  `json:"time_ms"`
	Locked     bool   `json:"locked"`
	BellLocked bool   `json:"bell_locked"`
	HasRung    bool   `json:"has_rung"`
}

type ScoreAdjustment struct {
	Slot  int `json:"slot"`
	Delta int `json:"delta"`
}

type ScoreAdjustmentSource struct {
	QuestionID   string `json:"question_id"`
	Version      int64  `json:"version"`
	Kind         string `json:"kind,omitempty"`
	kindProvided bool
}

func (s *ScoreAdjustmentSource) UnmarshalJSON(data []byte) error {
	type sourceAlias ScoreAdjustmentSource
	var decoded struct {
		*sourceAlias
		Kind json.RawMessage `json:"kind"`
	}
	decoded.sourceAlias = (*sourceAlias)(s)
	*s = ScoreAdjustmentSource{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	s.kindProvided = decoded.Kind != nil
	if s.kindProvided {
		return json.Unmarshal(decoded.Kind, &s.Kind)
	}
	return nil
}

type Match struct {
	ID                    string          `json:"id"`
	Name                  string          `json:"name"`
	Season                int             `json:"season"`
	Stage                 string          `json:"stage"`
	Status                MatchStatus     `json:"status"`
	QuestionSetID         string          `json:"question_set_id,omitempty"`
	QuestionSetRevisionID string          `json:"question_set_revision_id,omitempty"`
	Version               int64           `json:"version"`
	Result                json.RawMessage `json:"result"`
	Contestants           []Contestant    `json:"contestants"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
	StartedAt             *time.Time      `json:"started_at,omitempty"`
	CompletedAt           *time.Time      `json:"completed_at,omitempty"`
	ArchivedAt            *time.Time      `json:"archived_at,omitempty"`
	Selected              bool            `json:"selected"`
}

type CreateMatchInput struct {
	Name          string       `json:"name"`
	Season        int          `json:"season"`
	Stage         string       `json:"stage"`
	QuestionSetID string       `json:"question_set_id,omitempty"`
	Contestants   []Contestant `json:"contestants"`
}

type UpdateMatchInput struct {
	Name          string       `json:"name"`
	Season        int          `json:"season"`
	Stage         string       `json:"stage"`
	QuestionSetID string       `json:"question_set_id,omitempty"`
	Contestants   []Contestant `json:"contestants"`
	Version       int64        `json:"version"`
}

type QuestionSet struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Category          string     `json:"category"`
	CurrentRevisionID string     `json:"current_revision_id"`
	Revision          int64      `json:"revision"`
	Version           int64      `json:"version"`
	LinkedMatches     int        `json:"linked_matches"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	ArchivedAt        *time.Time `json:"archived_at,omitempty"`
}

type QuestionSetRevision struct {
	ID            string          `json:"id"`
	QuestionSetID string          `json:"question_set_id"`
	Revision      int64           `json:"revision"`
	Data          json.RawMessage `json:"data"`
	CreatedBy     int64           `json:"created_by,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type SaveQuestionSetInput struct {
	Name     string          `json:"name"`
	Category string          `json:"category"`
	Data     json.RawMessage `json:"data"`
	Version  int64           `json:"version,omitempty"`
}

type TimerState struct {
	IsRunning   bool  `json:"is_running"`
	IsPaused    bool  `json:"is_paused"`
	DurationMS  int64 `json:"duration_ms"`
	RemainingMS int64 `json:"remaining_ms"`
	ElapsedMS   int64 `json:"elapsed_ms"`
	StartedAt   int64 `json:"started_at"`
	// Kind is "question" (default), "steal" (Về đích 5 s) or "obstacle" (VCNV 15 s board timer).
	Kind string `json:"kind,omitempty"`
}

type RuntimeState struct {
	MatchID            string            `json:"match_id"`
	MatchName          string            `json:"match_name"`
	Season             int               `json:"season"`
	Stage              string            `json:"stage"`
	Epoch              int64             `json:"epoch"`
	Version            int64             `json:"version"`
	InputEpoch         int64             `json:"input_epoch"`
	BellEpoch          int64             `json:"bell_epoch"`
	Status             MatchStatus       `json:"status"`
	QuestionSetID      string            `json:"question_set_id,omitempty"`
	QuestionRevisionID string            `json:"question_revision_id,omitempty"`
	Controller         controller.State  `json:"controller"`
	Timer              TimerState        `json:"timer"`
	Contestants        []ContestantState `json:"contestants"`
	CurrentQuestion    *QuestionSnapshot `json:"current_question,omitempty"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

// QuestionSnapshot is copied into the Match runtime when an operator selects
// a question. It keeps an already revealed question stable even if the linked
// question set is revised during the event.
type QuestionSnapshot struct {
	ID                 string `json:"id"`
	Content            string `json:"content"`
	Answer             string `json:"answer,omitempty"`
	Type               string `json:"type"`
	AssetID            string `json:"asset_id,omitempty"`
	ExplanationAssetID string `json:"explanation_asset_id,omitempty"`
	Time               int    `json:"time,omitempty"`
	Keyword            string `json:"keyword,omitempty"`
	CharCount          int    `json:"char_count,omitempty"`
}

// VCNVDisplay contains only public display data: answer text appears only after
// its row is solved, and the obstacle appears only after the round is solved.
type VCNVDisplay struct {
	Obstacle        string   `json:"obstacle,omitempty"`
	ObstacleAssetID string   `json:"obstacle_asset_id"`
	ObstacleLength  int      `json:"obstacle_length"`
	RowLengths      []int    `json:"row_lengths"`
	RowAnswers      []string `json:"row_answers"`
}

type Command struct {
	MatchID            string `json:"match_id,omitempty"`
	ExpectedEpoch      int64  `json:"expected_epoch,omitempty"`
	ExpectedVersion    int64  `json:"expected_version,omitempty"`
	ExpectedInputEpoch int64  `json:"expected_input_epoch,omitempty"`
	ExpectedBellEpoch  int64  `json:"expected_bell_epoch,omitempty"`
	CommandID          string `json:"command_id,omitempty"`
}

type Event struct {
	MatchID   string          `json:"match_id"`
	Sequence  int64           `json:"sequence"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	ActorID   int64           `json:"actor_id,omitempty"`
	ClientID  string          `json:"client_id,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	State     *RuntimeState   `json:"state,omitempty"`
}

type AuditEntry struct {
	ID         int64           `json:"id"`
	ActorID    int64           `json:"actor_id,omitempty"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	MatchID    string          `json:"match_id,omitempty"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	Reason     string          `json:"reason,omitempty"`
	ClientID   string          `json:"client_id,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

type Actor struct {
	UserID   int64
	ClientID string
}

type Asset struct {
	ID           string    `json:"id"`
	SHA256       string    `json:"sha256"`
	OriginalName string    `json:"original_name"`
	MediaType    string    `json:"media_type"`
	Size         int64     `json:"size"`
	CreatedAt    time.Time `json:"created_at"`
}
