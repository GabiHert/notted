package domain

import "slices"

// ProjectDeletePlan is what deleting a project removes, for the confirm. Paths are archive-relative, slash form.
type ProjectDeletePlan struct {
	Key             string
	Files           []string // project file, the current profile's file for it, then every file under meetings/<key>/ sorted; never nil
	Folders         []string // meetings/<key>/ and its subfolders, each ending in "/", deepest first; never nil
	LocalMeetings   int      // Notted's meetings whose project is Key
	UnsavedMeetings int      // of those, the ones never saved to the archive (empty Folder)
	Recording       bool     // a meeting of the project is being recorded
}

// Same reports whether two plans list the same things. Recording is not compared.
func (p ProjectDeletePlan) Same(q ProjectDeletePlan) bool {
	return p.Key == q.Key && slices.Equal(p.Files, q.Files) && slices.Equal(p.Folders, q.Folders) &&
		p.LocalMeetings == q.LocalMeetings && p.UnsavedMeetings == q.UnsavedMeetings
}

// ProjectDeleteResult is what a project delete did.
type ProjectDeleteResult struct {
	DeletedMeetings int
	TrackedRemoved  []string // the removed Files git tracked before the delete, in Files order; never nil
}
