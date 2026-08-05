package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const installScriptURL = "https://raw.githubusercontent.com/Fume-shroom/agent-mission-handoff/main/install.sh"
const installPowerShellURL = "https://raw.githubusercontent.com/Fume-shroom/agent-mission-handoff/main/install.ps1"

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: amh doctor [--json]")
	}
	checks := doctorChecks()
	if *asJSON {
		body, err := json.MarshalIndent(checks, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(body))
		return doctorProblem(checks)
	}
	for _, check := range checks {
		fmt.Printf("%-7s %-18s %s\n", strings.ToUpper(check.Status), check.Name, check.Detail)
	}
	return doctorProblem(checks)
}

func doctorProblem(checks []doctorCheck) error {
	status := map[string]string{}
	for _, check := range checks {
		status[check.Name] = check.Status
	}
	if status["git"] != "ready" {
		return errors.New("AMH is not ready: git is required")
	}
	var installedAgents []string
	for _, agent := range []string{"codex", "claude"} {
		if status[agent] == "ready" {
			installedAgents = append(installedAgents, agent)
		}
	}
	if len(installedAgents) == 0 {
		return errors.New("AMH is not ready: install Codex or Claude Code")
	}
	for _, agent := range installedAgents {
		if status[agent+" skill"] != "ready" {
			return fmt.Errorf("AMH is not ready: %s Mission Handoff Skill is missing", agent)
		}
	}
	return nil
}

func doctorChecks() []doctorCheck {
	checks := []doctorCheck{{Name: "amh", Status: "ready", Detail: version}}
	if path, err := os.Executable(); err == nil {
		checks[0].Detail += " at " + path
	}
	checks = append(checks, commandCheck("git"), commandCheck("codex"), commandCheck("claude"))
	home, _ := os.UserHomeDir()
	for _, agent := range []string{"codex", "claude"} {
		skill := filepath.Join(home, "."+agent, "skills", "mission-handoff", "SKILL.md")
		checks = append(checks, pathCheck(agent+" skill", skill, false))
		sessions := filepath.Join(home, "."+agent, "sessions")
		if agent == "claude" {
			sessions = filepath.Join(home, ".claude", "projects")
		}
		checks = append(checks, pathCheck(agent+" sessions", sessions, true))
	}
	return checks
}

func commandCheck(name string) doctorCheck {
	path, err := exec.LookPath(name)
	if err != nil {
		return doctorCheck{Name: name, Status: "missing", Detail: "not found on PATH"}
	}
	return doctorCheck{Name: name, Status: "ready", Detail: path}
}

func pathCheck(name, path string, directory bool) doctorCheck {
	info, err := os.Stat(path)
	if err != nil || directory && !info.IsDir() || !directory && info.IsDir() {
		return doctorCheck{Name: name, Status: "missing", Detail: path}
	}
	return doctorCheck{Name: name, Status: "ready", Detail: path}
}

func runUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: amh update")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	installDir := firstNonEmpty(os.Getenv("AMH_INSTALL_DIR"), filepath.Dir(executable))
	if runtime.GOOS == "windows" {
		return startWindowsUpdate(installDir)
	}
	command := exec.Command("sh", "-c", `curl -fsSL "$1" | sh`, "amh-update", installScriptURL)
	command.Env = append(os.Environ(), "AMH_INSTALL_DIR="+installDir)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}

func startWindowsUpdate(installDir string) error {
	script := fmt.Sprintf(`$env:AMH_INSTALL_DIR='%s'; while (Get-Process -Id %d -ErrorAction SilentlyContinue) { Start-Sleep -Milliseconds 200 }; irm '%s' | iex`, powershellQuote(installDir), os.Getpid(), installPowerShellURL)
	command := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Start(); err != nil {
		return err
	}
	if err := command.Process.Release(); err != nil {
		return err
	}
	fmt.Println("AMH update started and will complete after this process exits.")
	return nil
}

func runUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: amh uninstall")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home: %w", err)
	}
	if err := validateUninstallHome(home); err != nil {
		return err
	}
	for _, path := range uninstallSkillDirs(home) {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		script := fmt.Sprintf(`$p='%s'; while (Get-Process -Id %d -ErrorAction SilentlyContinue) { Start-Sleep -Milliseconds 200 }; Remove-Item -LiteralPath $p -Force -ErrorAction SilentlyContinue`, powershellQuote(executable), os.Getpid())
		command := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
		if err := command.Start(); err != nil {
			return err
		}
		if err := command.Process.Release(); err != nil {
			return err
		}
		fmt.Println("Removed AMH Skills. The CLI will be removed after this process exits.")
		return nil
	}
	if err := os.Remove(executable); err != nil {
		return err
	}
	fmt.Printf("Removed %s and the Mission Handoff Skills.\n", executable)
	return nil
}

func validateUninstallHome(home string) error {
	home = filepath.Clean(strings.TrimSpace(home))
	if home == "." || home == "" || !filepath.IsAbs(home) {
		return errors.New("refusing to uninstall because the user home directory is unavailable or not absolute")
	}
	if filepath.Dir(home) == home {
		return errors.New("refusing to uninstall from a filesystem root")
	}
	return nil
}

func uninstallSkillDirs(home string) []string {
	return []string{
		filepath.Join(home, ".codex", "skills", "mission-handoff"),
		filepath.Join(home, ".claude", "skills", "mission-handoff"),
	}
}

func powershellQuote(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
