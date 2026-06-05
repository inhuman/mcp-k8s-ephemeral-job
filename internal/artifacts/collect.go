package artifacts

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"path"
)

// File — файл-артефакт или входной файл (примитивный тип, без зависимостей на другие пакеты).
type File struct {
	Name    string
	Mode    int64
	Size    int64
	Content []byte
}

// CollectFromTar читает tar-поток рабочей директории (из exec в sidecar) и собирает файлы под
// суммарным капом max. Превышение — truncated=true (FR-014: не молчаливая потеря, флаг + усечение).
// skip — базовое имя служебного файла (sentinel), которое не попадает в артефакты.
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

// BuildInputTar упаковывает входные файлы в tar для заливки в под (exec `tar xf -`).
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
