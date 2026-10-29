package wails

import "github.com/GabiHert/notted/internal/domain"

// ProjectDeletePlanDTO is what deleting a project removes, for the confirm.
type ProjectDeletePlanDTO struct {
	Key             string   `json:"key"`
	Files           []string `json:"files"`
	Folders         []string `json:"folders"`
	LocalMeetings   int      `json:"localMeetings"`
	UnsavedMeetings int      `json:"unsavedMeetings"`
	Recording       bool     `json:"recording"`
}

// ProjectDeleteResultDTO is what a project delete did.
type ProjectDeleteResultDTO struct {
	DeletedMeetings int      `json:"deletedMeetings"`
	TrackedRemoved  []string `json:"trackedRemoved"`
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func toProjectDeletePlanDTO(p domain.ProjectDeletePlan) ProjectDeletePlanDTO {
	return ProjectDeletePlanDTO{Key: p.Key, Files: nonNil(p.Files), Folders: nonNil(p.Folders), LocalMeetings: p.LocalMeetings, UnsavedMeetings: p.UnsavedMeetings, Recording: p.Recording}
}

func fromProjectDeletePlanDTO(p ProjectDeletePlanDTO) domain.ProjectDeletePlan {
	return domain.ProjectDeletePlan{Key: p.Key, Files: p.Files, Folders: p.Folders, LocalMeetings: p.LocalMeetings, UnsavedMeetings: p.UnsavedMeetings, Recording: p.Recording}
}

func toProjectDeleteResultDTO(r domain.ProjectDeleteResult) ProjectDeleteResultDTO {
	return ProjectDeleteResultDTO{DeletedMeetings: r.DeletedMeetings, TrackedRemoved: nonNil(r.TrackedRemoved)}
}

// GetProjectDeletePlan returns what DeleteProject would remove. It changes nothing.
func (a *App) GetProjectDeletePlan(key string) (ProjectDeletePlanDTO, error) {
	p, err := a.projects.DeletePlan(a.context(), key)
	if err != nil {
		return ProjectDeletePlanDTO{}, userFacing(err)
	}
	return toProjectDeletePlanDTO(p), nil
}

// DeleteProject deletes exactly what the plan lists, after checking it is still the same; it never commits.
func (a *App) DeleteProject(key string, plan ProjectDeletePlanDTO) (ProjectDeleteResultDTO, error) {
	r, err := a.projects.Delete(a.context(), key, fromProjectDeletePlanDTO(plan))
	if err != nil {
		return ProjectDeleteResultDTO{}, userFacing(err)
	}
	return toProjectDeleteResultDTO(r), nil
}
