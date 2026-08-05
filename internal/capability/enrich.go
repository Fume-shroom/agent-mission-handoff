package capability

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Fume-shroom/agent-mission-handoff/internal/capsule"
)

var cellarPath = regexp.MustCompile(`/Cellar/([^/]+)/([^/]+)/`)
var misePath = regexp.MustCompile(`/(?:mise|asdf)/installs/([^/]+)/([^/]+)/`)
var nodeModulePath = regexp.MustCompile(`^(.*?/node_modules/)((?:@[^/]+/)?[^/]+)/`)

func enrich(capability capsule.Capability, opts Options) capsule.Capability {
	switch capability.Kind {
	case "skill":
		if capability.Source == "" {
			capability.Source = findSkill(capability.Name, opts)
		}
		if capability.Source != "" {
			path, ok := resolveFile(capability.Source, opts.CWD)
			if !ok {
				capability.Source = ""
				break
			}
			capability.Source, capability.Version, capability.Digest = describeFile(path)
		}
	case "cli":
		path, err := exec.LookPath(capability.Name)
		if err == nil {
			capability.Source, capability.Version, capability.Digest = describeExecutable(path)
		}
	case "mcp":
		capability.Source, capability.Digest = describeMCP(capability.Name, opts)
	}
	return capability
}

func findSkill(name string, opts Options) string {
	home, _ := os.UserHomeDir()
	var roots []string
	if opts.CWD != "" {
		roots = append(roots,
			filepath.Join(opts.CWD, ".agents", "skills"),
			filepath.Join(opts.CWD, ".codex", "skills"),
			filepath.Join(opts.CWD, ".claude", "skills"),
		)
	}
	if opts.Home != "" {
		roots = append(roots, filepath.Join(opts.Home, "skills"), filepath.Join(opts.Home, "plugins", "cache"))
	} else {
		roots = append(roots,
			filepath.Join(home, ".agents", "skills"),
			filepath.Join(home, ".codex", "skills"),
			filepath.Join(home, ".codex", "plugins", "cache"),
			filepath.Join(home, ".claude", "skills"),
			filepath.Join(home, ".devflow-cli", "skills"),
		)
	}
	for _, root := range roots {
		direct := filepath.Join(root, name, "SKILL.md")
		if info, err := os.Stat(direct); err == nil && !info.IsDir() {
			return direct
		}
		found := ""
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || found != "" {
				return filepath.SkipDir
			}
			if !d.IsDir() && d.Name() == "SKILL.md" && filepath.Base(filepath.Dir(path)) == name {
				found = path
			}
			return nil
		})
		if found != "" {
			return found
		}
	}
	return ""
}

func describeFile(path string) (source, version, digest string) {
	path = filepath.Clean(path)
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	digest = fileDigest(path)
	dir := filepath.Dir(path)
	root, err := commandOutput(dir, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "file:" + path, "", digest
	}
	root = strings.TrimSpace(root)
	remote, _ := commandOutput(root, "git", "remote", "get-url", "origin")
	commit, _ := commandOutput(root, "git", "rev-parse", "HEAD")
	relative, _ := filepath.Rel(root, path)
	remote = strings.TrimSpace(remote)
	commit = strings.TrimSpace(commit)
	if remote == "" {
		return "file:" + path, commit, digest
	}
	return "git+" + remote + "#" + filepath.ToSlash(relative), commit, digest
}

// DescribeFile returns reproducibility metadata for an installed Skill file.
func DescribeFile(path string) (source, version, digest string) {
	return describeFile(path)
}

func describeExecutable(path string) (source, version, digest string) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = resolved
	}
	path = filepath.Clean(path)
	digest = fileDigest(path)
	unixPath := filepath.ToSlash(path)
	if match := cellarPath.FindStringSubmatch(unixPath); len(match) == 3 {
		return "homebrew:" + match[1], match[2], digest
	}
	if match := misePath.FindStringSubmatch(unixPath); len(match) == 3 {
		return "runtime:" + match[1], match[2], digest
	}
	if match := nodeModulePath.FindStringSubmatch(unixPath); len(match) == 3 {
		packageDir := filepath.FromSlash(match[1] + match[2])
		if body, err := os.ReadFile(filepath.Join(packageDir, "package.json")); err == nil {
			var metadata struct {
				Version string `json:"version"`
			}
			_ = json.Unmarshal(body, &metadata)
			return "npm:" + match[2], metadata.Version, digest
		}
	}
	return "path:" + path, "", digest
}

// DescribeExecutable returns reproducibility metadata for an installed CLI.
func DescribeExecutable(path string) (source, version, digest string) {
	return describeExecutable(path)
}

func resolveFile(path, cwd string) (string, bool) {
	if strings.HasPrefix(path, "~"+string(filepath.Separator)) || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~/"), "~"+string(filepath.Separator)))
		}
	}
	if !filepath.IsAbs(path) && cwd != "" {
		path = filepath.Join(cwd, path)
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	return path, err == nil && !info.IsDir()
}

func describeMCP(name string, opts Options) (source, digest string) {
	home, _ := os.UserHomeDir()
	root := opts.Home
	if root == "" {
		root = filepath.Join(home, "."+opts.Agent)
	}
	if opts.Agent == "codex" {
		for _, path := range []string{filepath.Join(opts.CWD, ".codex", "config.toml"), filepath.Join(root, "config.toml")} {
			body, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			section := tomlMCPSection(body, name)
			if len(section) > 0 {
				return "codex-config:" + path + "#mcp_servers." + name, bytesDigest(section)
			}
		}
		return "", ""
	}
	paths := []string{filepath.Join(opts.CWD, ".mcp.json"), filepath.Join(opts.CWD, ".claude", "settings.json"), filepath.Join(root, "settings.json")}
	if opts.Home == "" {
		paths = append(paths, filepath.Join(home, ".claude.json"))
	}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var value any
		if json.Unmarshal(body, &value) != nil {
			continue
		}
		if config := findMCPConfig(value, name); config != nil {
			encoded, _ := json.Marshal(config)
			return "claude-config:" + path + "#mcpServers." + name, bytesDigest(encoded)
		}
	}
	return "", ""
}

func tomlMCPSection(body []byte, name string) []byte {
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

func findMCPConfig(value any, name string) any {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if strings.EqualFold(key, "mcpServers") {
				if servers, ok := child.(map[string]any); ok {
					if config, exists := servers[name]; exists {
						return config
					}
				}
			}
			if config := findMCPConfig(child, name); config != nil {
				return config
			}
		}
	case []any:
		for _, child := range current {
			if config := findMCPConfig(child, name); config != nil {
				return config
			}
		}
	}
	return nil
}

func fileDigest(path string) string {
	body, err := os.ReadFile(path)
	if err != nil || len(body) > 64<<20 {
		return ""
	}
	return bytesDigest(body)
}

func bytesDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func commandOutput(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	body, err := cmd.Output()
	return string(body), err
}
