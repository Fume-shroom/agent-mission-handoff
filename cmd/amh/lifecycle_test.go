package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorJSONReportsInstalledAgentSkill(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	bin := t.TempDir()
	writeFakeCommand(t, bin, "git")
	writeFakeCommand(t, bin, "codex")
	t.Setenv("PATH", bin)
	skill := filepath.Join(home, ".codex", "skills", "mission-handoff", "SKILL.md")
	writeFile(t, skill, []byte("# skill\n"))
	out, err := captureOutput(t, func() error { return runDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var checks []doctorCheck
	if err := json.Unmarshal([]byte(out), &checks); err != nil {
		t.Fatal(err)
	}
	for _, check := range checks {
		if check.Name == "codex skill" {
			if check.Status != "ready" || check.Detail != skill {
				t.Fatalf("unexpected skill check: %+v", check)
			}
			return
		}
	}
	t.Fatal("codex skill check missing")
}

func TestDoctorProblemRequiresAnInstalledAgentAndSkill(t *testing.T) {
	if err := doctorProblem([]doctorCheck{{Name: "git", Status: "ready"}, {Name: "codex", Status: "missing"}, {Name: "claude", Status: "missing"}}); err == nil {
		t.Fatal("doctor accepted an environment without a coding Agent")
	}
	checks := []doctorCheck{
		{Name: "git", Status: "ready"},
		{Name: "codex", Status: "ready"},
		{Name: "codex skill", Status: "ready"},
		{Name: "claude", Status: "missing"},
	}
	if err := doctorProblem(checks); err != nil {
		t.Fatalf("doctor rejected a ready Codex installation: %v", err)
	}
}

func TestUninstallSkillDirsAreScoped(t *testing.T) {
	home := filepath.Clean("/home/tester")
	paths := uninstallSkillDirs(home)
	if len(paths) != 2 {
		t.Fatalf("paths = %v", paths)
	}
	for _, path := range paths {
		if !strings.HasPrefix(path, home+string(os.PathSeparator)) || filepath.Base(path) != "mission-handoff" {
			t.Fatalf("unsafe uninstall path: %s", path)
		}
	}
}

func TestValidateUninstallHomeRejectsUnsafeRoots(t *testing.T) {
	for _, home := range []string{"", ".", string(os.PathSeparator)} {
		if err := validateUninstallHome(home); err == nil {
			t.Fatalf("unsafe home %q was accepted", home)
		}
	}
	if err := validateUninstallHome(t.TempDir()); err != nil {
		t.Fatalf("normal absolute home was rejected: %v", err)
	}
}

func TestPowerShellQuote(t *testing.T) {
	if got := powershellQuote(`C:\Users\O'Brien\amh`); got != `C:\Users\O''Brien\amh` {
		t.Fatalf("quote = %q", got)
	}
}
