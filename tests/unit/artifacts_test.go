package unit

import (
	"archive/tar"
	"bytes"
	"testing"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/artifacts"
)

func tarWith(t *testing.T, files map[string][]byte) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf
}

func TestCollectFromTar_CapsTotal(t *testing.T) {
	blob := bytes.Repeat([]byte("x"), 1000)
	tb := tarWith(t, map[string][]byte{"a.txt": blob, "b.txt": blob, "c.txt": blob})

	files, truncated, err := artifacts.CollectFromTar(tb, 1500, ".runjob-ready")
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Errorf("expected truncated=true when files exceed the cap")
	}
	var total int64
	for _, f := range files {
		total += int64(len(f.Content))
	}
	if total > 1500 {
		t.Errorf("collected %d bytes, must not exceed cap 1500", total)
	}
}

func TestCollectFromTar_SkipsSentinel(t *testing.T) {
	tb := tarWith(t, map[string][]byte{".runjob-ready": {}, "out.txt": []byte("hi")})
	files, _, err := artifacts.CollectFromTar(tb, 1<<20, ".runjob-ready")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "out.txt" {
		t.Errorf("sentinel must be skipped, got %+v", files)
	}
}
