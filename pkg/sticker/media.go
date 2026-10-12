package sticker

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
)

// safeIDPattern restricts sticker IDs / set names to characters that are safe to
// use as file names, preventing path traversal via crafted IDs.
var safeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_\-]+$`)

// IsSafeID reports whether id is safe to embed in a file name.
func IsSafeID(id string) bool {
	return safeIDPattern.MatchString(strings.TrimSpace(id))
}

// FileToDataURL reads a local image file and returns a data URL whose MIME type
// is detected from the file's magic bytes (not from a hard-coded guess). This is
// required because Telegram stickers arrive in several formats (.webp for
// regular, .jpg thumbnails for animated/video) and a wrong MIME makes vision
// models reject the request.
func FileToDataURL(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("empty file: %s", path)
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		// Fall back to a permissive image type if sniffing fails; the model may
		// still cope, and callers surface the error otherwise.
		mime = "image/webp"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
