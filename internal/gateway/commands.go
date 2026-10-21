package gateway

import (
	"fmt"
	"sort"
	"time"

	"github.com/gomeznz/gpstrack/internal/h02"
)

// Preset is a named server-to-device command.
//
// The S20 relay commands are the forms commonly used by H02-family trackers.
// Which one the 4G CJ720 accepts over TCP is exactly what Phase 0 has to
// find out: send one, then watch the log for a V4 reply and check the relay
// with an SMS "status".
type Preset struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Danger      bool     `json:"danger"`
	Command     string   `json:"-"`
	Params      []string `json:"-"`
	Description string   `json:"description"`
}

var Presets = map[string]Preset{
	"cut": {
		Name: "cut", Label: "Cut fuel (S20 1,1)", Danger: true,
		Command: "S20", Params: []string{"1", "1"},
		Description: "Immobilise, short form",
	},
	"cut-alt": {
		Name: "cut-alt", Label: "Cut fuel (S20 long form)", Danger: true,
		Command: "S20", Params: []string{"1", "3", "10", "3", "5", "5", "3", "5", "3", "5", "3", "5"},
		Description: "Immobilise, long form some firmware wants",
	},
	"resume": {
		Name: "resume", Label: "Resume fuel (S20 0,0)",
		Command: "S20", Params: []string{"0", "0"},
		Description: "Restore",
	},
	"resume-alt": {
		Name: "resume-alt", Label: "Resume fuel (S20 1,0)",
		Command: "S20", Params: []string{"1", "0"},
		Description: "Restore, alternative form",
	},
}

// PresetList returns the presets sorted by name.
func PresetList() []Preset {
	out := make([]Preset, 0, len(Presets))
	for _, p := range Presets {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// BuildPreset formats a preset for a device.
func BuildPreset(name, deviceID string, now time.Time) (string, error) {
	p, ok := Presets[name]
	if !ok {
		return "", fmt.Errorf("unknown preset %q", name)
	}
	return h02.Command(deviceID, p.Command, p.Params, now), nil
}
