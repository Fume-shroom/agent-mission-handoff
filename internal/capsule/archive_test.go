package capsule

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Fume-shroom/agent-mission-handoff/internal/handoff"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mission.amh")
	want := Data{
		Manifest:      Manifest{Format: Format, CapsuleID: "cap-1", SourceAgent: "codex", SourceSessionID: "session-1"},
		Mission:       MissionCheckpoint{Objective: "debug timeout", Status: "in_progress", EvidenceTurnCount: 2},
		Capabilities:  []Capability{{Kind: "skill", Name: "logs", Detection: "observed", Confidence: 1, Required: true}},
		Workspace:     Workspace{CWD: "/work/repo", PathOnly: true},
		Session:       handoff.AgentSession{Format: handoff.IRFormat, SourceAgent: "codex", ThreadID: "session-1", CWD: "/work/repo", Conversation: []handoff.Turn{{Role: handoff.RoleUser, Text: "debug"}}},
		RawSession:    []byte("{\"type\":\"session_meta\"}\n"),
		WorktreePatch: []byte("diff --git a/a b/a\n"),
		IndexPatch:    []byte("diff --git a/a b/a\n"),
	}
	if err := Write(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Manifest.CapsuleID != want.Manifest.CapsuleID || got.Mission.Objective != want.Mission.Objective {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if string(got.RawSession) != string(want.RawSession) {
		t.Fatal("raw session changed")
	}
	if string(got.WorktreePatch) != string(want.WorktreePatch) {
		t.Fatal("worktree patch changed")
	}
	if string(got.IndexPatch) != string(want.IndexPatch) {
		t.Fatal("index patch changed")
	}
}

func TestReadsLegacyV1Capsule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.amh")
	data := Data{
		Manifest:  Manifest{Format: LegacyFormat, CapsuleID: "legacy", SourceAgent: "codex"},
		Mission:   MissionCheckpoint{Status: "in_progress"},
		Workspace: Workspace{PathOnly: true},
		Session:   handoff.AgentSession{Format: handoff.IRFormat, SourceAgent: "codex", Conversation: []handoff.Turn{{Role: handoff.RoleUser, Text: "debug"}}},
	}
	if err := Write(path, data); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Manifest.Format != LegacyFormat {
		t.Fatalf("format = %q, want legacy v1", got.Manifest.Format)
	}
}

func TestWriterRejectsEntriesThatReceiverWouldReject(t *testing.T) {
	if err := validateEntrySizes(map[string][]byte{"large": make([]byte, 5)}, 4, 10); err == nil {
		t.Fatal("oversized entry was accepted")
	}
	if err := validateEntrySizes(map[string][]byte{"a": make([]byte, 4), "b": make([]byte, 4)}, 4, 7); err == nil {
		t.Fatal("oversized total was accepted")
	}
}

func TestRejectsTamperedArchive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mission.amh")
	data := Data{
		Manifest:  Manifest{Format: Format, CapsuleID: "cap-1", SourceAgent: "codex"},
		Mission:   MissionCheckpoint{Objective: "original", Status: "in_progress"},
		Workspace: Workspace{PathOnly: true},
		Session:   handoff.AgentSession{Format: handoff.IRFormat, SourceAgent: "codex", Conversation: []handoff.Turn{{Role: handoff.RoleUser, Text: "debug"}}},
	}
	if err := Write(path, data); err != nil {
		t.Fatal(err)
	}
	tampered := filepath.Join(dir, "tampered.amh")
	rewriteZip(t, path, tampered, func(name string, body []byte) []byte {
		if name == "mission.json" {
			return []byte(`{"objective":"tampered","status":"in_progress"}`)
		}
		return body
	})
	if _, err := Read(tampered); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestRejectsPayloadMissingFromChecksumManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mission.amh")
	data := Data{
		Manifest:  Manifest{Format: Format, CapsuleID: "cap-1", SourceAgent: "codex"},
		Mission:   MissionCheckpoint{Objective: "original", Status: "in_progress"},
		Workspace: Workspace{PathOnly: true},
		Session:   handoff.AgentSession{Format: handoff.IRFormat, SourceAgent: "codex", Conversation: []handoff.Turn{{Role: handoff.RoleUser, Text: "debug"}}},
	}
	if err := Write(path, data); err != nil {
		t.Fatal(err)
	}
	tampered := filepath.Join(dir, "missing-checksum.amh")
	rewriteZip(t, path, tampered, func(name string, body []byte) []byte {
		if name != "checksums.json" {
			return body
		}
		var checksums map[string]string
		if err := json.Unmarshal(body, &checksums); err != nil {
			t.Fatal(err)
		}
		delete(checksums, "mission.json")
		body, err := json.Marshal(checksums)
		if err != nil {
			t.Fatal(err)
		}
		return body
	})
	if _, err := Read(tampered); err == nil {
		t.Fatal("expected missing checksum coverage to fail")
	}
}

func TestRejectsOptionalPatchMissingFromChecksumManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mission.amh")
	data := Data{
		Manifest:      Manifest{Format: Format, CapsuleID: "cap-1", SourceAgent: "codex"},
		Mission:       MissionCheckpoint{Objective: "original", Status: "in_progress"},
		Workspace:     Workspace{PathOnly: false, Dirty: true, PatchIncluded: true},
		Session:       handoff.AgentSession{Format: handoff.IRFormat, SourceAgent: "codex", Conversation: []handoff.Turn{{Role: handoff.RoleUser, Text: "debug"}}},
		WorktreePatch: []byte("diff --git a/a b/a\n"),
	}
	if err := Write(path, data); err != nil {
		t.Fatal(err)
	}
	tampered := filepath.Join(dir, "missing-patch-checksum.amh")
	rewriteZip(t, path, tampered, func(name string, body []byte) []byte {
		if name != "checksums.json" {
			return body
		}
		var checksums map[string]string
		if err := json.Unmarshal(body, &checksums); err != nil {
			t.Fatal(err)
		}
		delete(checksums, "workspace/changes.patch")
		body, err := json.Marshal(checksums)
		if err != nil {
			t.Fatal(err)
		}
		return body
	})
	if _, err := Read(tampered); err == nil {
		t.Fatal("expected optional patch without checksum coverage to fail")
	}
}

func TestRejectsPathTraversal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traversal.amh")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("../escape")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("bad"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("expected traversal archive to fail")
	}
}

func TestRejectsWindowsStyleTraversal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traversal.amh")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create(`..\escape`)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("bad"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("expected Windows traversal archive to fail")
	}
}

func rewriteZip(t *testing.T, source, destination string, mutate func(string, []byte) []byte) {
	t.Helper()
	zr, err := zip.OpenReader(source)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	f, err := os.Create(destination)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, entry := range zr.File {
		r, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.Create(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(mutate(entry.Name, body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
