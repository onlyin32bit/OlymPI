package sound

import "time"

const OutputRoom = "sound-output"

type Bus string

const (
	BusMusic   Bus = "music"
	BusEffects Bus = "effects"
	// BusTimer is a single slot: a new timer playback replaces the previous one.
	BusTimer Bus = "timer"
)

// Legacy loudness ratio effects .5 : timer .4 : music .4 : question audio .6,
// normalised so question audio plays at full volume.
const (
	VolumeEffects       = 0.5 / 0.6
	VolumeTimer         = 0.4 / 0.6
	VolumeMusic         = 0.4 / 0.6
	VolumeQuestionAudio = 1.0
)

func ValidBus(bus Bus) bool {
	return bus == BusMusic || bus == BusEffects || bus == BusTimer
}

type Sound struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	SortOrder   int       `json:"sort_order"`
	Bus         Bus       `json:"bus"`
	Volume      float64   `json:"volume"`
	Loop        bool      `json:"loop"`
	FadeInMS    int       `json:"fade_in_ms"`
	FadeOutMS   int       `json:"fade_out_ms"`
	BuiltinPath string    `json:"-"`
	AssetID     string    `json:"asset_id,omitempty"`
	Builtin     bool      `json:"builtin"`
	Enabled     bool      `json:"enabled"`
	URL         string    `json:"url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Update struct {
	Name      string  `json:"name"`
	Category  string  `json:"category"`
	SortOrder int     `json:"sort_order"`
	Bus       Bus     `json:"bus"`
	Volume    float64 `json:"volume"`
	Loop      bool    `json:"loop"`
	FadeInMS  int     `json:"fade_in_ms"`
	FadeOutMS int     `json:"fade_out_ms"`
	Enabled   bool    `json:"enabled"`
}

type PlayRequest struct {
	SoundID string `json:"sound_id"`
}

type StopRequest struct {
	PlaybackID string `json:"playback_id,omitempty"`
	FadeMS     int    `json:"fade_ms,omitempty"`
}

type StopBusRequest struct {
	Bus    Bus `json:"bus"`
	FadeMS int `json:"fade_ms,omitempty"`
}

type TestRequest struct {
	Scope string `json:"scope"`
}

type Playback struct {
	PlaybackID string  `json:"playback_id"`
	SoundID    string  `json:"sound_id"`
	URL        string  `json:"url"`
	Bus        Bus     `json:"bus"`
	Volume     float64 `json:"volume"`
	Loop       bool    `json:"loop"`
	FadeInMS   int     `json:"fade_in_ms"`
	FadeOutMS  int     `json:"fade_out_ms"`
	StartAt    int64   `json:"start_at"`
}

type StopEvent struct {
	PlaybackID string `json:"playback_id,omitempty"`
	Bus        Bus    `json:"bus,omitempty"`
	All        bool   `json:"all,omitempty"`
	FadeMS     int    `json:"fade_ms"`
}

type Status struct {
	OutputCount int       `json:"output_count"`
	Music       *Playback `json:"music,omitempty"`
}
