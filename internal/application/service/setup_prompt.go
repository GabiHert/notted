package service

import (
	"bytes"
	"context"
	_ "embed"
	"slices"
	"strings"
	"text/template"
)

//go:embed setup_prompt.tmpl
var setupPromptText string

var setupPromptTmpl = template.Must(template.New("setup").Funcs(template.FuncMap{"join": strings.Join}).Parse(setupPromptText))

// setupPromptData is everything the template reads; an empty field prints as "not chosen yet" or is left out.
type setupPromptData struct {
	Archive              string
	ArchiveProblem       string
	Ready                bool
	Profile              string
	Projects             []string
	ProjectsError        string
	ProfileFiles         []string
	ProfileProjectsError string
	Missing              []string
}

// GetSetupPrompt returns the text the person pastes into Claude Code to finish the setup. It is built from a fixed
// template and the saved settings; an unset archive or profile, or an archive that cannot be read, is described in
// the text and never fails the call. Only an unreadable settings store does.
func (s *SetupService) GetSetupPrompt(ctx context.Context) (string, error) {
	st, err := s.settings.GetSettings(ctx)
	if err != nil {
		return "", err
	}
	d := setupPromptData{Archive: st.CheckoutPath, Profile: st.Profile}
	if d.Archive != "" {
		if err := checkArchive(d.Archive); err != nil {
			d.ArchiveProblem = err.Error()
		}
		d.Ready = d.ArchiveProblem == ""
		if d.Ready {
			d.fillProjects()
			if d.Profile != "" {
				d.fillProfile()
			}
		}
	}
	var buf bytes.Buffer
	if err := setupPromptTmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (d *setupPromptData) fillProjects() {
	keys, err := listMarkdownKeys(d.Archive, "instructions/projects")
	if err != nil {
		d.ProjectsError = err.Error()
		return
	}
	d.Projects = keys
}

func (d *setupPromptData) fillProfile() {
	base := "instructions/people/" + d.Profile
	for _, rel := range []string{base + ".md", base + ".local.md"} {
		has, err := instructionExists(d.Archive, rel)
		if err != nil {
			d.ProfileProjectsError = err.Error()
			return
		}
		if has {
			d.ProfileFiles = append(d.ProfileFiles, rel)
		}
	}
	mine, err := listMarkdownKeys(d.Archive, base)
	if err != nil {
		d.ProfileProjectsError = err.Error()
		return
	}
	for _, k := range mine {
		d.ProfileFiles = append(d.ProfileFiles, base+"/"+k+".md")
	}
	for _, k := range d.Projects {
		if !slices.Contains(mine, k) {
			d.Missing = append(d.Missing, k)
		}
	}
}
