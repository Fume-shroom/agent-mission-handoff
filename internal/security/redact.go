package security

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
)

type Report struct {
	Count int
	Kinds []string
}

type rule struct {
	name        string
	pattern     *regexp.Regexp
	replacement []byte
}

var rules = []rule{
	{
		name:        "url-userinfo",
		pattern:     regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`),
		replacement: []byte("${1}[REDACTED]@"),
	},
	{
		name:        "private-key",
		pattern:     regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`),
		replacement: []byte("[REDACTED PRIVATE KEY]"),
	},
	{
		name:        "authorization-bearer",
		pattern:     regexp.MustCompile(`(?i)(authorization[\\\"']*\s*[:=]\s*[\\\"']*bearer\s+)[A-Za-z0-9._~+/=-]{12,}`),
		replacement: []byte("${1}[REDACTED]"),
	},
	{
		name:        "secret-assignment",
		pattern:     regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|auth[_-]?token|password|passwd|client[_-]?secret|secret)[\\\"']*\s*[:=]\s*[\\\"']*)[^\s\\\"',}]{8,}`),
		replacement: []byte("${1}[REDACTED]"),
	},
	{
		name:        "known-token",
		pattern:     regexp.MustCompile(`(?:github_pat_[A-Za-z0-9_]{20,}|gh[pousr]_[A-Za-z0-9]{20,}|sk-[A-Za-z0-9_-]{20,}|xox[baprs]-[A-Za-z0-9-]{12,}|AKIA[0-9A-Z]{16})`),
		replacement: []byte("[REDACTED TOKEN]"),
	},
}

// Redact removes high-confidence credential shapes from serialized session or
// patch data. It is intentionally conservative: the report is a best-effort
// safety signal, not proof that arbitrary conversation text contains no secret.
func Redact(body []byte) ([]byte, Report) {
	out := append([]byte(nil), body...)
	report := Report{}
	for _, current := range rules {
		var count int
		out, count = applyRule(out, current)
		if count == 0 {
			continue
		}
		report.Count += count
		report.Kinds = append(report.Kinds, current.name)
	}
	sort.Strings(report.Kinds)
	return out, report
}

func applyRule(body []byte, current rule) ([]byte, int) {
	matches := current.pattern.FindAllSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return body, 0
	}
	out := make([]byte, 0, len(body))
	last := 0
	for _, match := range matches {
		out = append(out, body[last:match[0]]...)
		out = current.pattern.Expand(out, current.replacement, body, match)
		last = match[1]
	}
	out = append(out, body[last:]...)
	return out, len(matches)
}

// RedactPatch preserves unified-diff applicability when secrets appear only in
// added text. Matches in headers, context, removed lines, or private-key blocks
// make the patch unsafe to rewrite and therefore require omission.
func RedactPatch(body []byte) ([]byte, Report, bool) {
	_, report := Redact(body)
	if report.Count == 0 {
		return append([]byte(nil), body...), report, true
	}
	if containsKind(report, "private-key") {
		return nil, report, false
	}
	var out bytes.Buffer
	inBinaryPatch := false
	for _, line := range bytes.SplitAfter(body, []byte{'\n'}) {
		if bytes.HasPrefix(line, []byte("diff --git ")) {
			inBinaryPatch = false
		}
		if bytes.HasPrefix(line, []byte("GIT binary patch")) {
			inBinaryPatch = true
		}
		added := !inBinaryPatch && bytes.HasPrefix(line, []byte{'+'}) && !bytes.HasPrefix(line, []byte("+++"))
		if added {
			redacted, _ := Redact(line[1:])
			out.WriteByte('+')
			out.Write(redacted)
			continue
		}
		if _, lineReport := Redact(line); lineReport.Count > 0 {
			return nil, report, false
		}
		out.Write(line)
	}
	return out.Bytes(), report, true
}

func containsKind(report Report, kind string) bool {
	for _, current := range report.Kinds {
		if current == kind {
			return true
		}
	}
	return false
}

// RedactJSON applies the same best-effort policy to structured metadata while
// preserving its concrete Go type.
func RedactJSON[T any](value T) (T, Report, error) {
	var zero T
	body, err := json.Marshal(value)
	if err != nil {
		return zero, Report{}, err
	}
	body, report := Redact(body)
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		return zero, Report{}, err
	}
	return out, report, nil
}

func (r *Report) Add(other Report) {
	r.Count += other.Count
	r.Kinds = append(r.Kinds, other.Kinds...)
	sort.Strings(r.Kinds)
}
