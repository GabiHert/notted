package speech

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GabiHert/phonon/internal/application/port"
)

// ErrModelInUse is returned when asked to remove a folder the current models use.
var ErrModelInUse = errors.New("That model is in use and cannot be removed.")

// used is the set of top-level names in the models folder the manifest installs into.
func (m *ModelManager) used() (map[string]bool, error) {
	manifest, err := loadManifest()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{WhisperDirName: true, VADDirName: true}
	for _, a := range manifest.Models {
		out[a.Dir] = true
	}
	return out, nil
}

// Unused lists the files and folders directly in the models folder that no manifest artifact
// installs into, such as the sherpa-onnx models of earlier versions, with their sizes. Hidden
// entries (the download scratch folder) and symbolic links are left out.
func (m *ModelManager) Unused() ([]port.UnusedModel, error) {
	used, err := m.used()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(m.modelsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return []port.UnusedModel{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []port.UnusedModel{}
	for _, e := range entries {
		name := e.Name()
		if used[name] || strings.HasPrefix(name, ".") || e.Type()&fs.ModeSymlink != 0 {
			continue
		}
		out = append(out, port.UnusedModel{Name: name, Bytes: diskSize(filepath.Join(m.modelsDir, name))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// RemoveUnused deletes one entry Unused lists. It refuses a name that is not a plain entry of
// the models folder, one the manifest uses, a hidden one, a symbolic link, and any removal while
// a download runs.
func (m *ModelManager) RemoveUnused(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || filepath.Base(name) != name {
		return fmt.Errorf("%q is not a model in the models folder", name)
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("%q is not a model in the models folder", name)
	}
	m.mu.Lock()
	running := m.running
	m.mu.Unlock()
	if running {
		return ErrDownloadRunning
	}
	used, err := m.used()
	if err != nil {
		return err
	}
	if used[name] {
		return ErrModelInUse
	}
	root, err := filepath.EvalSymlinks(m.modelsDir)
	if err != nil {
		return err
	}
	p := filepath.Join(root, name)
	if filepath.Dir(p) != root {
		return fmt.Errorf("%q is not a model in the models folder", name)
	}
	st, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if st.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%q is a link, not a model folder", name)
	}
	return os.RemoveAll(p)
}

// diskSize sums the sizes of the regular files under p, without following links.
func diskSize(p string) int64 {
	var total int64
	_ = filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
