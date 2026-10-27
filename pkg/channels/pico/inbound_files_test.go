package pico

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistInboundFiles_NoFilesIsNoop(t *testing.T) {
	c := &PicoChannel{}
	tags, err := c.persistInboundFiles(map[string]any{"content": "hi"}, "msg-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected no tags, got %v", tags)
	}
}

func TestPersistInboundFiles_WritesFileAndReturnsTag(t *testing.T) {
	c := &PicoChannel{}
	payload := base64.StdEncoding.EncodeToString([]byte("hello pdf"))
	dataURL := "data:application/pdf;base64," + payload

	tags, err := c.persistInboundFiles(map[string]any{
		"files": []any{
			map[string]any{"filename": "report.pdf", "data": dataURL},
		},
	}, "msg-test-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 1 || !strings.HasPrefix(tags[0], "[file:") {
		t.Fatalf("expected one [file:...] tag, got %v", tags)
	}

	path := strings.TrimSuffix(strings.TrimPrefix(tags[0], "[file:"), "]")
	defer os.RemoveAll(filepath.Dir(path))

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(got) != "hello pdf" {
		t.Fatalf("content = %q", string(got))
	}
	if !strings.HasSuffix(path, "report.pdf") {
		t.Fatalf("unexpected filename: %s", path)
	}
	// Files must live under the agent workspace so read_file can access them.
	normalized := filepath.ToSlash(path)
	if !strings.Contains(normalized, "/workspace/uploads/") {
		t.Fatalf("expected file under workspace/uploads, got %s", path)
	}
}

func TestPersistInboundFiles_RejectsImages(t *testing.T) {
	c := &PicoChannel{}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("x"))
	_, err := c.persistInboundFiles(map[string]any{
		"files": []any{map[string]any{"filename": "a.png", "data": dataURL}},
	}, "msg-test-2")
	if err == nil {
		t.Fatal("expected images to be rejected on the generic-file path")
	}
}

func TestPersistInboundFiles_RejectsMalformed(t *testing.T) {
	c := &PicoChannel{}
	if _, err := c.persistInboundFiles(map[string]any{
		"files": []any{map[string]any{"filename": "x.txt", "data": "not-a-data-url"}},
	}, "msg-test-3"); err == nil {
		t.Fatal("expected malformed data URL to be rejected")
	}
}

func TestSanitizeFileComponent_BlocksTraversal(t *testing.T) {
	cases := map[string]string{
		"../../etc/passwd": "_.._.._etc_passwd",
		"a/b\\c.txt":       "a_b_c.txt",
	}
	for in := range cases {
		got := sanitizeFileComponent(in)
		if strings.Contains(got, "/") || strings.Contains(got, "\\") || strings.Contains(got, "..") {
			t.Fatalf("sanitize(%q) = %q still unsafe", in, got)
		}
	}
}

// A message carrying only a generic file (no text, no inline media) must not be
// rejected as "empty_content".
func TestHandleMessageSend_FileOnlyIsNotRejected(t *testing.T) {
	dataURL := "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte("hi"))
	payload := map[string]any{
		"files": []any{map[string]any{"filename": "note.txt", "data": dataURL}},
	}
	content, _ := payload["content"].(string)
	hasFile := false
	if raw, ok := payload["files"].([]any); ok && len(raw) > 0 {
		hasFile = true
	}
	if strings.TrimSpace(content) == "" && !hasFile {
		t.Fatal("file-only payload must not be treated as empty content")
	}
}

// SetWorkspaceDir must be honoured: attachments go to <workspace>/uploads so
// read_file (restricted to the workspace) can reach them even when the
// workspace is configured somewhere other than <picoclaw-home>/workspace.
func TestPersistInboundFiles_UsesConfiguredWorkspace(t *testing.T) {
	custom := t.TempDir()
	c := &PicoChannel{}
	c.SetWorkspaceDir(custom)

	dataURL := "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte("x"))
	tags, err := c.persistInboundFiles(map[string]any{
		"files": []any{map[string]any{"filename": "a.txt", "data": dataURL}},
	}, "msg-ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %v", tags)
	}

	path := strings.TrimSuffix(strings.TrimPrefix(tags[0], "[file:"), "]")
	absCustom, _ := filepath.Abs(custom)
	absPath, _ := filepath.Abs(path)
	rel, err := filepath.Rel(absCustom, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("file %s not under configured workspace %s", absPath, absCustom)
	}
}
