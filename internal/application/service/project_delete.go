package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

const (
	msgProjectChoose  = "Choose a project to delete."
	msgProjectStop    = "Stop the recording before you delete this project."
	msgProjectFinish  = " Delete the project again to finish."
	msgProjectNothing = "There is nothing to delete for project %q."
)

type meetingDeleter interface {
	Delete(ctx context.Context, meetingID string) error
}

// ProjectDeleteDeps are the collaborators of ProjectDeleteService.
type ProjectDeleteDeps struct {
	Store    port.MeetingStore
	Settings port.SettingsStore
	Recorder activeMeeting
	Meetings meetingDeleter // *DeleteService
	Git      port.Git
	Emit     port.EventEmitter
}

// ProjectDeleteService deletes a project: its files and meetings folder in the archive, its meetings in Notted and its root setting.
type ProjectDeleteService struct{ d ProjectDeleteDeps }

// NewProjectDeleteService builds the project delete service.
func NewProjectDeleteService(d ProjectDeleteDeps) *ProjectDeleteService {
	return &ProjectDeleteService{d: d}
}

// DeletePlan lists what Delete would remove for key. It changes nothing and runs no git.
func (s *ProjectDeleteService) DeletePlan(ctx context.Context, key string) (domain.ProjectDeletePlan, error) {
	k := strings.TrimSpace(key)
	if k == "" {
		return domain.ProjectDeletePlan{}, errors.New(msgProjectChoose)
	}
	if !keyRe.MatchString(k) {
		return domain.ProjectDeletePlan{}, errProjectKey
	}
	st, err := s.d.Settings.GetSettings(ctx)
	if err != nil {
		return domain.ProjectDeletePlan{}, err
	}
	if st.CheckoutPath == "" {
		return domain.ProjectDeletePlan{}, errNoArchive
	}

	files := []string{}
	rels := []string{"instructions/projects/" + k + ".md"}
	if st.Profile != "" && keyRe.MatchString(st.Profile) {
		rels = append(rels, "instructions/people/"+st.Profile+"/"+k+".md")
	}
	for _, rel := range rels {
		found, err := regularFileExists(st.CheckoutPath, rel)
		if err != nil {
			return domain.ProjectDeletePlan{}, err
		}
		if found {
			files = append(files, rel)
		}
	}

	folders, meetingFiles, err := meetingsTree(st.CheckoutPath, k)
	if err != nil {
		return domain.ProjectDeletePlan{}, err
	}
	plan := domain.ProjectDeletePlan{Key: k, Files: append(files, meetingFiles...), Folders: folders}

	ms, err := s.d.Store.ListMeetings(ctx)
	if err != nil {
		return domain.ProjectDeletePlan{}, err
	}
	for _, m := range ms {
		if m.ProjectKey != k {
			continue
		}
		plan.LocalMeetings++
		if m.Folder == "" {
			plan.UnsavedMeetings++
		}
		if m.State == domain.StateRecording || s.d.Recorder.ActiveMeetingID() == m.ID {
			plan.Recording = true
		}
	}
	if len(plan.Files) == 0 && len(plan.Folders) == 0 && plan.LocalMeetings == 0 {
		return domain.ProjectDeletePlan{}, fmt.Errorf(msgProjectNothing, k)
	}
	return plan, nil
}

// regularFileExists reports whether rel is a regular file in the archive; a link or special file is refused.
func regularFileExists(checkout, rel string) (bool, error) {
	realRoot, err := confine(checkout, rel)
	if err != nil {
		return false, err
	}
	root, err := os.OpenRoot(realRoot)
	if err != nil {
		return false, err
	}
	defer root.Close()
	fi, err := root.Lstat(filepath.FromSlash(rel))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, mapRootErr(err)
	case fi.Mode()&fs.ModeSymlink != 0:
		return false, errOutside
	case !fi.Mode().IsRegular():
		return false, notRegularErr(rel)
	}
	return true, nil
}

// meetingsTree lists the folders (deepest first) and the sorted files under meetings/<key>/.
func meetingsTree(checkout, key string) (folders, files []string, err error) {
	folders, files = []string{}, []string{}
	realRoot, err := confineMeetings(checkout, key)
	if err != nil {
		return nil, nil, err
	}
	root, err := os.OpenRoot(realRoot)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	dir := "meetings/" + key
	if _, err := root.Lstat(filepath.FromSlash(dir)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return folders, files, nil
		}
		return nil, nil, mapRootErr(err)
	}
	entries, err := walkProfileDir(root, dir, []string{dir + "/"})
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		if strings.HasSuffix(e, "/") {
			folders = append(folders, e)
		} else {
			files = append(files, e)
		}
	}
	sort.Strings(files)
	sort.SliceStable(folders, func(i, j int) bool {
		if a, b := strings.Count(folders[i], "/"), strings.Count(folders[j], "/"); a != b {
			return a > b
		}
		return folders[i] < folders[j]
	})
	return folders, files, nil
}

// confineMeetings resolves the archive folder through its own links and refuses a link, or a non-folder,
// at meetings/ or meetings/<key>. A missing component ends the walk without error.
func confineMeetings(checkout, key string) (realRoot string, err error) {
	realRoot, err = filepath.EvalSymlinks(checkout)
	if err != nil {
		return "", err
	}
	for _, rel := range []string{"meetings", "meetings/" + key} {
		fi, lerr := os.Lstat(filepath.Join(realRoot, filepath.FromSlash(rel)))
		switch {
		case errors.Is(lerr, fs.ErrNotExist):
			return realRoot, nil
		case lerr != nil:
			return "", lerr
		case fi.Mode()&fs.ModeSymlink != 0:
			return "", errOutside
		case !fi.IsDir():
			return "", fmt.Errorf("Notted cannot delete %s: it is not a folder.", rel)
		}
	}
	return realRoot, nil
}

// Delete removes exactly what plan lists, after checking it is still what DeletePlan returns.
// Local meetings go first, then the archive files and folders, then the root setting.
func (s *ProjectDeleteService) Delete(ctx context.Context, key string, plan domain.ProjectDeletePlan) (domain.ProjectDeleteResult, error) {
	cur, err := s.DeletePlan(ctx, key)
	if err != nil {
		return domain.ProjectDeleteResult{}, err
	}
	if cur.Recording {
		return domain.ProjectDeleteResult{}, errors.New(msgProjectStop)
	}
	if !cur.Same(plan) {
		return domain.ProjectDeleteResult{}, fmt.Errorf("Project %q changed since you reviewed it. Review the list again before deleting.", cur.Key)
	}
	st, err := s.d.Settings.GetSettings(ctx)
	if err != nil {
		return domain.ProjectDeleteResult{}, err
	}
	tracked := s.tracked(ctx, st.CheckoutPath, cur.Files)

	ms, err := s.d.Store.ListMeetings(ctx)
	if err != nil {
		return domain.ProjectDeleteResult{}, err
	}
	deleted := 0
	for _, m := range ms {
		if m.ProjectKey != cur.Key {
			continue
		}
		if err := s.d.Meetings.Delete(ctx, m.ID); err != nil {
			if !errors.Is(err, ErrAudioLeft) {
				return domain.ProjectDeleteResult{}, fmt.Errorf("Notted could not delete the meeting %q: %s"+msgProjectFinish, m.Title, err.Error())
			}
			log.Printf("project delete: %v", err)
		}
		deleted++
		s.d.Emit.Emit("meeting:deleted", map[string]any{"meetingId": m.ID})
	}

	reversed := slices.Clone(cur.Folders)
	slices.Reverse(reversed)
	if err := removeListed(st.CheckoutPath, slices.Concat(cur.Files, reversed)); err != nil {
		return domain.ProjectDeleteResult{}, errors.New(err.Error() + msgProjectFinish)
	}

	st, err = s.d.Settings.GetSettings(ctx)
	if err != nil {
		return domain.ProjectDeleteResult{}, err
	}
	if _, ok := st.ProjectRoots[cur.Key]; ok {
		delete(st.ProjectRoots, cur.Key)
		if err := s.d.Settings.SaveSettings(ctx, st); err != nil {
			return domain.ProjectDeleteResult{}, fmt.Errorf("The project's files are deleted, but its root folder setting could not be cleared: %v", err)
		}
	}
	s.d.Emit.Emit("project:deleted", map[string]any{"key": cur.Key})
	return domain.ProjectDeleteResult{DeletedMeetings: deleted, TrackedRemoved: tracked}, nil
}

// tracked returns, in files order, the files git tracks. It runs the one read-only command, git ls-files;
// a folder that is not a git repository gives an empty list.
func (s *ProjectDeleteService) tracked(ctx context.Context, checkout string, files []string) []string {
	if len(files) == 0 {
		return []string{}
	}
	out, err := s.d.Git.Run(ctx, checkout, append([]string{"ls-files", "-z", "--"}, files...)...)
	if err != nil {
		log.Printf("project delete: git ls-files: %v", err)
		return []string{}
	}
	set := map[string]bool{}
	for _, p := range strings.Split(out, "\x00") {
		set[p] = true
	}
	res := []string{}
	for _, f := range files {
		if set[f] {
			res = append(res, f)
		}
	}
	return res
}
