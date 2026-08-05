package capsule

import "github.com/Fume-shroom/agent-mission-handoff/internal/handoff"

const Format = "agent-mission-handoff-v1"

type Manifest struct {
	Format          string `json:"format"`
	CapsuleID       string `json:"capsule_id"`
	CreatedAt       string `json:"created_at"`
	SourceAgent     string `json:"source_agent"`
	SourceSessionID string `json:"source_session_id,omitempty"`
	SourcePath      string `json:"source_path,omitempty"`
}

type MissionCheckpoint struct {
	Objective         string   `json:"objective,omitempty"`
	Status            string   `json:"status"`
	CurrentSummary    string   `json:"current_summary,omitempty"`
	Completed         []string `json:"completed,omitempty"`
	CurrentHypotheses []string `json:"current_hypotheses,omitempty"`
	NextActions       []string `json:"next_actions,omitempty"`
	InterruptedAction string   `json:"interrupted_action,omitempty"`
	EvidenceTurnCount int      `json:"evidence_turn_count"`
}

type Capability struct {
	Kind       string  `json:"kind"`
	Name       string  `json:"name"`
	Version    string  `json:"version,omitempty"`
	Source     string  `json:"source,omitempty"`
	Digest     string  `json:"digest,omitempty"`
	Detection  string  `json:"detection"`
	Confidence float64 `json:"confidence"`
	Required   bool    `json:"required"`
}

type Workspace struct {
	CWD      string           `json:"cwd,omitempty"`
	Git      *handoff.GitInfo `json:"git,omitempty"`
	PathOnly bool             `json:"path_only"`
}

type Data struct {
	Manifest     Manifest             `json:"manifest"`
	Mission      MissionCheckpoint    `json:"mission"`
	Capabilities []Capability         `json:"capabilities"`
	Workspace    Workspace            `json:"workspace"`
	Session      handoff.AgentSession `json:"session"`
	RawSession   []byte               `json:"-"`
}
