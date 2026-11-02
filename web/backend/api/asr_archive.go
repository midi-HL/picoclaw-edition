package api

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// extractAnyArchive extracts tar.bz2 / tar.gz / tgz / zip into destDir.
//
// The format is detected from the file's magic bytes rather than its name,
// because download URLs frequently have no extension or a misleading one.
func extractAnyArchive(archivePath, workDir, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	head := make([]byte, 6)
	n, _ := io.ReadFull(f, head)
	f.Close()
	head = head[:n]

	switch {
	case n >= 2 && head[0] == 'B' && head[1] == 'Z':
		return extractTar(archivePath, destDir, func(r io.Reader) (io.Reader, error) {
			return bzip2.NewReader(r), nil
		})
	case n >= 2 && head[0] == 0x1f && head[1] == 0x8b:
		return extractTar(archivePath, destDir, func(r io.Reader) (io.Reader, error) {
			return gzip.NewReader(r)
		})
	case n >= 4 && string(head[:4]) == "PK\x03\x04":
		return extractZip(archivePath, destDir)
	case n >= 2 && head[0] == 'R' && head[1] == 'I':
		return fmt.Errorf("downloaded file is a WAV, not a model archive")
	}

	// Fall back to raw tar (uncompressed).
	return extractTar(archivePath, destDir, func(r io.Reader) (io.Reader, error) { return r, nil })
}

// decompressor wraps a reader; returning (nil, err) signals failure.
type decompressor func(io.Reader) (io.Reader, error)

func extractTar(archivePath, destDir string, decompress decompressor) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	dr, err := decompress(f)
	if err != nil {
		return fmt.Errorf("decompress: %w", err)
	}

	tr := tar.NewReader(dr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		rel := filepath.Clean(filepath.FromSlash(hdr.Name))
		if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			continue // skip traversal / root entries
		}
		outPath := filepath.Join(destDir, rel)
		if !strings.HasPrefix(outPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(outPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				return err
			}
			if err := writeFileFrom(outPath, tr, hdr.FileInfo().Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

func extractZip(archivePath, destDir string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	for _, zf := range zr.File {
		rel := filepath.Clean(filepath.FromSlash(zf.Name))
		if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			continue
		}
		outPath := filepath.Join(destDir, rel)
		if !strings.HasPrefix(outPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(outPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}

		rc, err := zf.Open()
		if err != nil {
			return err
		}
		err = writeFileFrom(outPath, rc, zf.Mode())
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeFileFrom(path string, r io.Reader, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm()|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
