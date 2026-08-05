package restore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Fume-shroom/agent-mission-handoff/internal/capability"
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
				_, version, digest := capability.DescribeFile(path)
				applyIdentityCheck(&check, c, version, digest)
			}
		case "mcp":
			if digest, ok := mcps[c.Name]; ok {
				check.Status = "ready"
				applyIdentityCheck(&check, c, "", digest)
			} else {
				check.Detail = "configure this MCP in the destination agent"
			}
		case "cli":
			if path, err := exec.LookPath(c.Name); err == nil {
				check.Status, check.Detail = "ready", path
				_, version, digest := capability.DescribeExecutable(path)
				applyIdentityCheck(&check, c, version, digest)
			}
		default:
			check.Status = "deferred"
			check.Detail = "validated by the destination agent when invoked"
		}
		checks = append(checks, check)
	}
	if data.Workspace.Dirty {
		patchBytes := len(data.WorktreePatch) + len(data.IndexPatch)
		if patchBytes > 0 {
			checks = append(checks, Check{Kind: "workspace", Name: "workspace-patch", Status: "deferred", Detail: fmt.Sprintf("%d bytes of source workspace changes are available after user confirmation", patchBytes)})
		} else {
			checks = append(checks, Check{Kind: "workspace", Name: "workspace-patch", Status: "missing", Detail: firstDetail(data.Workspace.PatchOmission, data.Workspace.IndexPatchOmission, "source had uncommitted changes but no patch was included"), Required: true})
		}
	}
	return checks
}

func applyIdentityCheck(check *Check, expected capsule.Capability, actualVersion, actualDigest string) {
	// An exact content match is the strongest available identity signal, even
	// when a binary does not expose a parseable version string.
	if expected.Digest != "" && actualDigest != "" && expected.Digest == actualDigest {
		return
	}
	var differences []string
	if expected.Kind == "cli" && expected.Version != "" {
		if actualVersion == "" {
			differences = append(differences, "source version could not be verified locally")
		} else if expected.Version != actualVersion {
			differences = append(differences, fmt.Sprintf("version differs: destination %s, source %s", actualVersion, expected.Version))
		} else {
			return
		}
	}
	if expected.Digest != "" {
		if actualDigest == "" {
			differences = append(differences, "source content digest could not be verified locally")
		} else if expected.Digest != actualDigest {
			differences = append(differences, "content digest differs from the source capability")
		} else if len(differences) == 0 {
			return
		}
	} else if expected.Version != "" && expected.Kind != "cli" {
		if actualVersion == "" {
			differences = append(differences, "source version could not be verified locally")
		} else if expected.Version != actualVersion {
			differences = append(differences, fmt.Sprintf("version differs: destination %s, source %s", actualVersion, expected.Version))
		}
	}
	if len(differences) == 0 {
		return
	}
	check.Status = "different"
	if check.Detail != "" {
		check.Detail += "; "
	}
	check.Detail += strings.Join(differences, "; ")
}

func firstDetail(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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

func installedMCPs(cwd, target, overrideHome string) map[string]string {
	out := map[string]string{}
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
				collectMCPConfigs(value, out)
			}
		}
	}
	return out
}

func collectCodexMCPs(body []byte, out map[string]string) {
	re := regexp.MustCompile(`(?m)^\[mcp_servers\.([A-Za-z0-9._-]+)\]`)
	for _, match := range re.FindAllSubmatch(body, -1) {
		name := string(match[1])
		out[name] = digestBytes(codexMCPSection(body, name))
	}
}

func codexMCPSection(body []byte, name string) []byte {
	lines := strings.Split(string(body), "\n")
	header := "[mcp_servers." + name + "]"
	var out []string
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			if inSection {
				break
			}
			inSection = trimmed == header
		}
		if inSection {
			out = append(out, line)
		}
	}
	return []byte(strings.Join(out, "\n"))
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

func collectMCPConfigs(v any, out map[string]string) {
	switch x := v.(type) {
	case map[string]any:
		for key, child := range x {
			if strings.EqualFold(key, "mcpServers") {
				if servers, ok := child.(map[string]any); ok {
					for name, config := range servers {
						encoded, _ := json.Marshal(config)
						out[name] = digestBytes(encoded)
					}
				}
			}
			collectMCPConfigs(child, out)
		}
	case []any:
		for _, child := range x {
			collectMCPConfigs(child, out)
		}
	}
}

func digestBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func FormatChecks(checks []Check) string {
	sort.SliceStable(checks, func(i, j int) bool { return checks[i].Status < checks[j].Status })
	var b strings.Builder
	for _, c := range checks {
		mark := map[string]string{"ready": "OK", "missing": "MISSING", "different": "DIFF", "deferred": "DEFER"}[c.Status]
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
