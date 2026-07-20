package artifacts

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"path"
)

// File is an artifact or an input file (a primitive type with no dependencies on other packages).
type File struct {
	Name    string
	Mode    int64
	Size    int64
	Content []byte
}

// CollectFromTar reads the tar stream of the working directory (from an exec into the
// sidecar) and gathers files under the total cap max. Going over sets truncated=true —
// loss is never silent, the flag accompanies the truncation.
// skip is the base name of the internal sentinel file, which never becomes an artifact.
func CollectFromTar(r io.Reader, max int64, skip string) ([]File, bool, error) {
	tr := tar.NewReader(r)
	var files []File
	var total int64
	truncated := false

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false, fmt.Errorf("read tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := path.Clean(hdr.Name)
		if path.Base(name) == skip {
			continue
		}

		remaining := max - total
		if remaining <= 0 {
			truncated = true
			break
		}

		var buf bytes.Buffer
		n, err := io.CopyN(&buf, tr, remaining)
		if err != nil && err != io.EOF {
			return nil, false, fmt.Errorf("read tar entry %q: %w", name, err)
		}
		total += n
		if hdr.Size > n {
			truncated = true
		}
		files = append(files, File{Name: name, Size: hdr.Size, Content: buf.Bytes()})
	}
	return files, truncated, nil
}

// BuildInputTar packs the input files into a tar for upload into the pod (exec `tar xf -`).
func BuildInputTar(files []File) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, f := range files {
		mode := f.Mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &tar.Header{Name: f.Name, Mode: mode, Size: int64(len(f.Content))}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("tar header %q: %w", f.Name, err)
		}
		if _, err := tw.Write(f.Content); err != nil {
			return nil, fmt.Errorf("tar write %q: %w", f.Name, err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("tar close: %w", err)
	}
	return buf.Bytes(), nil
}
