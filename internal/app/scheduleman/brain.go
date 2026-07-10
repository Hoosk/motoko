package scheduleman

import (
	"encoding/json"
	"strings"
	"time"
)

type persistedSchedule struct {
	ID          string `json:"id"`
	Instruction string `json:"instruction"`
	Interval    string `json:"interval"`
	OneShot     bool   `json:"one_shot,omitempty"`
}

func FormatScheduleBrain(defs []Definition) string {
	var sb strings.Builder
	sb.WriteString("# Schedule\n")
	for _, def := range defs {
		encoded, err := json.Marshal(persistedSchedule{
			ID:          def.ID,
			Instruction: def.Instruction,
			Interval:    def.Interval.String(),
			OneShot:     def.OneShot,
		})
		if err != nil {
			continue
		}
		sb.Write(encoded)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func ParseScheduleBrain(content string) []Definition {
	lines := strings.Split(content, "\n")
	defs := make([]Definition, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var persisted persistedSchedule
		if err := json.Unmarshal([]byte(line), &persisted); err != nil {
			continue
		}
		if strings.TrimSpace(persisted.ID) == "" || strings.TrimSpace(persisted.Instruction) == "" {
			continue
		}
		interval, err := time.ParseDuration(strings.TrimSpace(persisted.Interval))
		if err != nil {
			continue
		}
		defs = append(defs, Definition{
			ID:          strings.TrimSpace(persisted.ID),
			Instruction: strings.TrimSpace(persisted.Instruction),
			Interval:    interval,
			OneShot:     persisted.OneShot,
		})
	}
	return defs
}
