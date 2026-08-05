package restore

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Fume-shroom/agent-mission-handoff/internal/capsule"
)

type Check struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Required bool   `json:"required,omitempty"`
}

type Options struct {
	CWD    string
	Target string
	Home   string
}

func Preflight(data capsule.Data, cwd string) []Check {
	return PreflightFor(data, Options{CWD: cwd})
}

func PreflightFor(data capsule.Data, opts Options) []Check {
	checks := []Check{}
	cwd := opts.CWD
	if cwd == "" {
		cwd = data.Workspace.CWD
	}
	if cwd == "" {
		checks = append(checks, Check{Kind: "workspace", Name: "cwd", Status: "missing", Detail: "no target path selected", Required: true})
	} else if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		checks = append(checks, Check{Kind: "workspace", Name: cwd, Status: "missing", Detail: "target directory does not exist", Required: true})
	} else {
		checks = append(checks, Check{Kind: "workspace", Name: cwd, Status: "ready"})
		checks = append(checks, gitWorkspaceChecks(data, cwd)...)
	}

	mcps := installedMCPs(cwd, opts.Target, opts.Home)
	skills := installedSkills(cwd, opts.Target, opts.Home)
	for _, c := range data.Capabilities {
		check := Check{Kind: c.Kind, Name: c.Name, Status: "missing", Required: c.Required}
		switch c.Kind {
		case "skill":
			if path := skills[c.Name]; path != "" {
				check.Status, check.Detail = "ready", path
			}
		case "mcp":
			if mcps[c.Name] {
				check.Status = "ready"
			} else {
				check.Detail = "configure this MCP in the destination agent"
			}
		case "cli":
			if path, err := exec.LookPath(c.Name); err == nil {
				check.Status, check.Detail = "ready", path
			}
		default:
			check.Status = "deferred"
			check.Detail = "validated by the destination agent when invoked"
		}
		checks = append(checks, check)
	}
	return checks
}

func installedSkills(cwd, target, overrideHome string) map[string]string {
	home, _ := os.UserHomeDir()
	roots := []string{
		filepath.Join(cwd, ".agents", "skills"),
	}
	if target == "" || target == "codex" {
		root := filepath.Join(home, ".codex")
		if overrideHome != "" {
			root = overrideHome
		}
		roots = append(roots, filepath.Join(cwd, ".codex", "skills"), filepath.Join(root, "skills"), filepath.Join(root, "plugins", "cache"))
	}
	if target == "" || target == "claude" {
		root := filepath.Join(home, ".claude")
		if overrideHome != "" {
			root = overrideHome
		}
		roots = append(roots, filepath.Join(cwd, ".claude", "skills"), filepath.Join(root, "skills"))
	}
	if overrideHome == "" {
		roots = append(roots, filepath.Join(home, ".agents", "skills"), filepath.Join(home, ".devflow-cli", "skills"))
	}
	out := map[string]string{}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if d.Name() == "SKILL.md" {
				name := filepath.Base(filepath.Dir(path))
				if _, exists := out[name]; !exists {
					out[name] = path
				}
			}
			return nil
		})
	}
	return out
}

func installedMCPs(cwd, target, overrideHome string) map[string]bool {
	out := map[string]bool{}
	home, _ := os.UserHomeDir()
	if target == "" || target == "codex" {
		root := filepath.Join(home, ".codex")
		if overrideHome != "" {
			root = overrideHome
		}
		if body, err := os.ReadFile(filepath.Join(root, "config.toml")); err == nil {
			collectCodexMCPs(body, out)
		}
		if body, err := os.ReadFile(filepath.Join(cwd, ".codex", "config.toml")); err == nil {
			collectCodexMCPs(body, out)
		}
	}
	if target != "" && target != "claude" {
		return out
	}
	root := filepath.Join(home, ".claude")
	paths := []string{filepath.Join(home, ".claude.json"), filepath.Join(root, "settings.json")}
	if overrideHome != "" {
		root = overrideHome
		paths = []string{filepath.Join(root, "settings.json"), filepath.Join(root, "config.json")}
	}
	paths = append(paths, filepath.Join(cwd, ".mcp.json"), filepath.Join(cwd, ".claude", "settings.json"))
	for _, path := range paths {
		if body, err := os.ReadFile(path); err == nil {
			var value any
			if json.Unmarshal(body, &value) == nil {
				collectMCPKeys(value, out)
			}
		}
	}
	return out
}

func collectCodexMCPs(body []byte, out map[string]bool) {
	re := regexp.MustCompile(`(?m)^\[mcp_servers\.([A-Za-z0-9._-]+)\]`)
	for _, match := range re.FindAllSubmatch(body, -1) {
		out[string(match[1])] = true
	}
}

func gitWorkspaceChecks(data capsule.Data, cwd string) []Check {
	if data.Workspace.Git == nil {
		return nil
	}
	if err := exec.Command("git", "-C", cwd, "rev-parse", "--git-dir").Run(); err != nil {
		return []Check{{Kind: "workspace", Name: "git", Status: "missing", Detail: "target is not a Git workspace", Required: true}}
	}
	git := data.Workspace.Git
	if git.Remote != "" {
		body, err := exec.Command("git", "-C", cwd, "remote", "get-url", "origin").Output()
		if err != nil || normalizeRemote(string(body)) != normalizeRemote(git.Remote) {
			return []Check{{Kind: "workspace", Name: "git-remote", Status: "missing", Detail: "target origin does not match source mission", Required: true}}
		}
	}
	if git.Commit != "" {
		body, err := exec.Command("git", "-C", cwd, "rev-parse", "HEAD").Output()
		if err != nil || strings.TrimSpace(string(body)) != git.Commit {
			return []Check{{Kind: "workspace", Name: "git-head", Status: "missing", Detail: "target HEAD does not match the source commit", Required: true}}
		}
	} else if git.Branch != "" {
		body, err := exec.Command("git", "-C", cwd, "branch", "--show-current").Output()
		if err != nil || strings.TrimSpace(string(body)) != git.Branch {
			return []Check{{Kind: "workspace", Name: "git-branch", Status: "missing", Detail: "target branch does not match the source mission", Required: true}}
		}
	}
	return []Check{{Kind: "workspace", Name: "git", Status: "ready"}}
}

func normalizeRemote(remote string) string {
	remote = strings.TrimSpace(remote)
	remote = strings.TrimSuffix(strings.TrimSuffix(remote, "/"), ".git")
	if strings.HasPrefix(remote, "git@") {
		if host, path, ok := strings.Cut(strings.TrimPrefix(remote, "git@"), ":"); ok {
			return strings.ToLower(host) + "/" + strings.TrimPrefix(path, "/")
		}
	}
	if parsed, err := url.Parse(remote); err == nil && parsed.Host != "" {
		return strings.ToLower(parsed.Hostname()) + "/" + strings.TrimPrefix(strings.TrimSuffix(parsed.Path, ".git"), "/")
	}
	return remote
}

func collectMCPKeys(v any, out map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for key, child := range x {
			if strings.EqualFold(key, "mcpServers") {
				if servers, ok := child.(map[string]any); ok {
					for name := range servers {
						out[name] = true
					}
				}
			}
			collectMCPKeys(child, out)
		}
	case []any:
		for _, child := range x {
			collectMCPKeys(child, out)
		}
	}
}

func FormatChecks(checks []Check) string {
	sort.SliceStable(checks, func(i, j int) bool { return checks[i].Status < checks[j].Status })
	var b strings.Builder
	for _, c := range checks {
		mark := map[string]string{"ready": "OK", "missing": "MISSING", "deferred": "DEFER"}[c.Status]
		fmt.Fprintf(&b, "%-7s %-10s %s", mark, SafeTerminal(c.Kind), SafeTerminal(c.Name))
		if c.Detail != "" {
			fmt.Fprintf(&b, " - %s", SafeTerminal(c.Detail))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func SafeTerminal(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r >= 0x80 && r <= 0x9f {
			return -1
		}
		return r
	}, value)
}
