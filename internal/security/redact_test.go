package security

import (
	"bytes"
	"fmt"
	"testing"
)

func TestRedactKnownSecrets(t *testing.T) {
	body := []byte(`{"authorization":"Bearer abcdefghijklmnopqrstuvwxyz","api_key":"sk-abcdefghijklmnopqrstuvwxyz123456"}`)
	got, report := Redact(body)
	if bytes.Contains(got, []byte("abcdefghijklmnopqrstuvwxyz")) {
		t.Fatalf("secret remained in %s", got)
	}
	if report.Count != 2 {
		t.Fatalf("redactions = %d, want 2", report.Count)
	}
}

func TestRedactLeavesNormalConversationAlone(t *testing.T) {
	body := []byte(`{"message":"debug the token refresh flow"}`)
	got, report := Redact(body)
	if !bytes.Equal(got, body) || report.Count != 0 {
		t.Fatalf("normal text changed: %s, %+v", got, report)
	}
}

func TestRedactURLUserInfo(t *testing.T) {
	body := []byte(`{"remote":"https://user:secret-token@example.com/org/repo.git"}`)
	got, report := Redact(body)
	if bytes.Contains(got, []byte("secret-token")) || !bytes.Contains(got, []byte("https://[REDACTED]@example.com")) {
		t.Fatalf("credential-bearing URL was not redacted: %s", got)
	}
	if report.Count != 1 {
		t.Fatalf("redactions = %d, want 1", report.Count)
	}
}

func TestRedactJSONPreservesType(t *testing.T) {
	type metadata struct {
		Remote string `json:"remote"`
	}
	got, report, err := RedactJSON(metadata{Remote: "https://token@example.com/repo.git"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Remote != "https://[REDACTED]@example.com/repo.git" || report.Count != 1 {
		t.Fatalf("unexpected structured redaction: %+v, %+v", got, report)
	}
}

func TestRedactPatchRewritesSensitiveAddedLines(t *testing.T) {
	patch := []byte("diff --git a/config.txt b/config.txt\n--- a/config.txt\n+++ b/config.txt\n@@ -1 +1,2 @@\n base=true\n+api_key=abcdefghijklmnopqrstuvwxyz123456\n")
	got, report, safe := RedactPatch(patch)
	if !safe || report.Count != 1 {
		t.Fatalf("safe = %t, report = %+v", safe, report)
	}
	if bytes.Contains(got, []byte("abcdefghijklmnopqrstuvwxyz")) || !bytes.Contains(got, []byte("+api_key=[REDACTED]")) {
		t.Fatalf("added secret was not safely redacted:\n%s", got)
	}
}

func TestRedactPatchRejectsSensitiveContext(t *testing.T) {
	patch := []byte("diff --git a/config.txt b/config.txt\n--- a/config.txt\n+++ b/config.txt\n@@ -1,2 +1,2 @@\n api_key=abcdefghijklmnopqrstuvwxyz123456\n-old=true\n+new=true\n")
	got, report, safe := RedactPatch(patch)
	if safe || got != nil || report.Count != 1 {
		t.Fatalf("unsafe context was accepted: safe=%t report=%+v patch=%q", safe, report, got)
	}
}

func TestRedactPatchRejectsPrivateKeyBlock(t *testing.T) {
	begin := "-----BEGIN " + "PRIVATE KEY-----"
	end := "-----END " + "PRIVATE KEY-----"
	patch := []byte(fmt.Sprintf("diff --git a/key.pem b/key.pem\nnew file mode 100644\n--- /dev/null\n+++ b/key.pem\n@@ -0,0 +1,3 @@\n+%s\n+secret\n+%s\n", begin, end))
	got, report, safe := RedactPatch(patch)
	if safe || got != nil || !containsKind(report, "private-key") {
		t.Fatalf("private key patch was accepted: safe=%t report=%+v", safe, report)
	}
}
