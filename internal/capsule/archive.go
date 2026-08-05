package capsule

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxEntrySize = 128 << 20
	maxTotalSize = 256 << 20
)

var allowedEntries = map[string]bool{
	"manifest.json":           true,
	"mission.json":            true,
	"capabilities.json":       true,
	"workspace.json":          true,
	"session/normalized.json": true,
	"session/source.jsonl":    true,
	"workspace/changes.patch": true,
	"workspace/index.patch":   true,
	"checksums.json":          true,
}

func Write(path string, data Data) error {
	entries := map[string][]byte{}
	var err error
	entries["manifest.json"], err = marshal(data.Manifest)
	if err != nil {
		return err
	}
	entries["mission.json"], err = marshal(data.Mission)
	if err != nil {
		return err
	}
	entries["capabilities.json"], err = marshal(data.Capabilities)
	if err != nil {
		return err
	}
	entries["workspace.json"], err = marshal(data.Workspace)
	if err != nil {
		return err
	}
	entries["session/normalized.json"], err = marshal(data.Session)
	if err != nil {
		return err
	}
	entries["session/source.jsonl"] = data.RawSession
	if len(data.WorktreePatch) > 0 {
		entries["workspace/changes.patch"] = data.WorktreePatch
	}
	if len(data.IndexPatch) > 0 {
		entries["workspace/index.patch"] = data.IndexPatch
	}

	checksums := map[string]string{}
	for name, body := range entries {
		sum := sha256.Sum256(body)
		checksums[name] = hex.EncodeToString(sum[:])
	}
	entries["checksums.json"], err = marshal(checksums)
	if err != nil {
		return err
	}
	if err := validateEntrySizes(entries, maxEntrySize, maxTotalSize); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".amh-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(tmpName)
		}
	}()

	zw := zip.NewWriter(tmp)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			return err
		}
		if _, err := w.Write(entries[name]); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func validateEntrySizes(entries map[string][]byte, entryLimit, totalLimit int) error {
	total := 0
	for name, body := range entries {
		if len(body) > entryLimit {
			return fmt.Errorf("capsule entry %q exceeds limit", name)
		}
		total += len(body)
		if total > totalLimit {
			return errors.New("capsule exceeds total uncompressed size limit")
		}
	}
	return nil
}

func Read(path string) (Data, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Data{}, err
	}
	defer zr.Close()
	entries := map[string][]byte{}
	totalSize := 0
	for _, f := range zr.File {
		if !safeName(f.Name) {
			return Data{}, fmt.Errorf("unsafe capsule entry %q", f.Name)
		}
		if !allowedEntries[f.Name] {
			return Data{}, fmt.Errorf("unexpected capsule entry %q", f.Name)
		}
		if _, exists := entries[f.Name]; exists {
			return Data{}, fmt.Errorf("duplicate capsule entry %q", f.Name)
		}
		if f.UncompressedSize64 > maxEntrySize {
			return Data{}, fmt.Errorf("capsule entry %q is too large", f.Name)
		}
		r, err := f.Open()
		if err != nil {
			return Data{}, err
		}
		body, readErr := io.ReadAll(io.LimitReader(r, maxEntrySize+1))
		r.Close()
		if readErr != nil {
			return Data{}, readErr
		}
		if len(body) > maxEntrySize {
			return Data{}, fmt.Errorf("capsule entry %q exceeds limit", f.Name)
		}
		totalSize += len(body)
		if totalSize > maxTotalSize {
			return Data{}, errors.New("capsule exceeds total uncompressed size limit")
		}
		entries[f.Name] = body
	}

	var checksums map[string]string
	if err := json.Unmarshal(entries["checksums.json"], &checksums); err != nil {
		return Data{}, fmt.Errorf("checksums: %w", err)
	}
	for _, name := range []string{"manifest.json", "mission.json", "capabilities.json", "workspace.json", "session/normalized.json", "session/source.jsonl"} {
		if _, ok := entries[name]; !ok {
			return Data{}, fmt.Errorf("missing capsule entry %q", name)
		}
		if _, ok := checksums[name]; !ok {
			return Data{}, fmt.Errorf("missing checksum for %q", name)
		}
	}
	for name := range entries {
		if name == "checksums.json" {
			continue
		}
		if _, ok := checksums[name]; !ok {
			return Data{}, fmt.Errorf("missing checksum for %q", name)
		}
	}
	if _, ok := checksums["checksums.json"]; ok {
		return Data{}, errors.New("checksums.json must not checksum itself")
	}
	for name, want := range checksums {
		body, ok := entries[name]
		if !ok {
			return Data{}, fmt.Errorf("missing capsule entry %q", name)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != want {
			return Data{}, fmt.Errorf("checksum mismatch for %q", name)
		}
	}

	var data Data
	if err := decode(entries, "manifest.json", &data.Manifest); err != nil {
		return Data{}, err
	}
	if data.Manifest.Format != Format && data.Manifest.Format != LegacyFormat {
		return Data{}, fmt.Errorf("unsupported capsule format %q", data.Manifest.Format)
	}
	if err := decode(entries, "mission.json", &data.Mission); err != nil {
		return Data{}, err
	}
	if err := decode(entries, "capabilities.json", &data.Capabilities); err != nil {
		return Data{}, err
	}
	if err := decode(entries, "workspace.json", &data.Workspace); err != nil {
		return Data{}, err
	}
	if err := decode(entries, "session/normalized.json", &data.Session); err != nil {
		return Data{}, err
	}
	data.RawSession = entries["session/source.jsonl"]
	data.WorktreePatch = entries["workspace/changes.patch"]
	data.IndexPatch = entries["workspace/index.patch"]
	return data, nil
}

func marshal(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func decode(entries map[string][]byte, name string, dst any) error {
	body, ok := entries[name]
	if !ok {
		return fmt.Errorf("missing capsule entry %q", name)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func safeName(name string) bool {
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") {
		return false
	}
	clean := pathpkg.Clean(name)
	return clean == name && clean != ".." && !strings.HasPrefix(clean, "../")
}
