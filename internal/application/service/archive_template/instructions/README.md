# Custom instructions

## What they are

Everything about a project, a person's tools, where existing work lives and what "accept" does is a custom instruction file in this folder. Files are plain Markdown. Notted puts them into the session's system prompt as text and reads only the field marked *parsed* below.

## Layers and precedence

For a meeting of project `<key>` run by profile `<p>`, the prompt is assembled in this order:

1. `prompts/base.md`
2. `instructions/projects/<key>.md`
3. `instructions/people/<p>.md`
4. `instructions/people/<p>/<key>.md` (if present)
5. `instructions/people/<p>.local.md` (if present)

The base's non-negotiables win over every layer. Otherwise the more specific layer wins (person over project over base), except that the project file wins on facts about the project. No other project's file is ever loaded.

## Project file

`instructions/projects/<key>.md`. `<key>` matches `^[a-z0-9][a-z0-9-]*$`. Use these `##` headings, in this order:

- `## Key`: the key, one line.
- `## Name`: the project's name, one line.
- `## Aliases`: one `- <alias>` bullet per name that may appear in a meeting title.
- `## About`: who the project is for and what it does, a few sentences.
- `## Existing work`: bullets naming where existing tasks live, with paths relative to the person's project root for this project and the identifier each source uses. May say "None"; then every suggestion is `new`.

```markdown
## Key
acme

## Name
Acme Corp

## Aliases
- Acme
- ACME Industries

## About
Acme builds warehouse robots. We are delivering their fleet dashboard.

## Existing work
- `tasks/*.md`: one file per task, identified by its file name.
```

## Person file

`instructions/people/<profile>.md`. It is project-neutral: it is loaded for every project, so it names no project. Sections: `## Role`, `## Style` (language, phrasing, size and number of suggestions), `## Tools` (tools used for every project).

```markdown
## Role
Engineer on the delivery team.

## Style
English. Short, imperative task titles. At most five suggestions.

## Tools
Markdown task files in the project root.
```

## Per-project person file

`instructions/people/<profile>/<key>.md`. It is loaded only for that project's meetings.

- `## Project root`: one line, the local path of this project (`~/` allowed). *Parsed*: only this directory is granted to the session. Absent means no extra directory.
- `## Tools`: tools the person uses for this project.
- `## On accept`: what accepting a suggestion does for this project. Absent means accepting prints the task text and does nothing else.

```markdown
## Project root
~/work/globex-portal

## Tools
Task files under `tasks/`.

## On accept
Draft a new file in `tasks/` and show it before saving.
```

## Local overrides

`instructions/people/<profile>.local.md` is git-ignored and loaded last. Keep it project-neutral.

## The profile

Your profile names your person file (for example `ana`). Choose it in Notted's settings. It never falls back to another person's file.

## Limits

An instruction cannot widen the base's non-negotiables. Nothing is created, sent or committed without the person's yes, and a message to another session is a trigger for its reader, never an approval.
