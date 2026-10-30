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
- `## Existing work`: where this project's existing tasks live, naming only what every teammate can read: the issue tracker (team and identifier), GitHub repositories (`owner/repo` and the folder in it), and free text. One bullet per source, with the identifier it uses. May say "None"; then every suggestion is `new`. This file is shared with the whole team, so never name a local path or anything only you can open; put those in your per-project file.

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
- GitHub `acme/dashboard`: one issue per task, identified by `#<n>`.
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
- `## Existing work`: sources only you can read, as paths relative to your `## Project root`, with the identifier each uses. Claude reads them together with the project file's.
- `## On accept`: what accepting a suggestion does for this project. Absent means accepting prints the task text and does nothing else.

```markdown
## Project root
~/work/globex-portal

## Tools
Task files under `tasks/`.

## Existing work
- `tasks/*.md`: one file per task, identified by its file name.

## On accept
Draft a new file in `tasks/` and show it before saving.
```

## Local overrides

`instructions/people/<profile>.local.md` is git-ignored and loaded last. Keep it project-neutral.

## The profile

Your profile names your person file (for example `ana`). Choose it in Notted's settings. It never falls back to another person's file.

## Forms in Notted

When a session runs inside Notted, Claude has an `ask` tool. It shows a form at the top of the meeting's page, under **Waiting for you**, and waits for your answer. A form can hold:

- `text`: Markdown, rendered;
- `html`: a static HTML view, with no scripts and no network;
- `choice`: one option out of several, one of them marked recommended;
- `multi`: any number of options, for example the suggestions to accept;
- `input`: a text field;
- `confirm`: a yes and a no button.

Every form also has a **Note** for free text and a **Not now** button. **Not now** means no: Claude does not act on a dismissed form. In the turns Notted runs without a terminal, such as **Review suggestions**, Claude Code's permission requests arrive as cards too: **Allow** lets the action run, **Deny** refuses it. Your instructions can ask for a form, for example "ask me with a `choice` which folder to use".

## Deleting a project

In Notted, open **Settings**, click **Delete a project…** under **Projects**, then **Delete…** on the project's row. Notted removes from this folder:

- `instructions/projects/<key>.md`;
- your own `instructions/people/<profile>/<key>.md`;
- the whole `meetings/<key>/` folder, with every saved meeting of the project.

It also deletes the project's meetings from Notted. A meeting that was never saved here is gone for good.

Teammates' files for the project, `instructions/people/<other>/<key>.md`, stay. They are unused leftovers: a per-project person file is loaded only for its project's meetings. Each person can delete their own.

Notted then offers one commit, `project(<key>): delete`, with only the removed files, and pushes it. Nothing is rewritten: to restore the project, check its files out from the commit before that one, for example `git checkout <commit>^ -- instructions/projects/<key>.md meetings/<key>`.

## Limits

An instruction cannot widen the base's non-negotiables. Nothing is created, sent or committed without the person's yes, and a message to another session is a trigger for its reader, never an approval.
