---
name: meetings
description: Process a recorded meeting folder in this archive. Writes the summary with action items, saves it to git on the person's yes, suggests tasks checked against the existing-work sources the instructions name, and records which suggestions the person accepts. Use for "process", "suggest" or "accept" on a folder under meetings/.
---

# Meetings

This skill turns one meeting folder into a summary, suggested tasks and recorded decisions. The
session's instructions (the base rules, the project file, and the person's files) carry everything
specific: what the sources are, which tools read them, what happens on accept. Follow them; this
skill names none of their content.

A meeting folder holds `meeting.md` (front matter: `date, project, title, participants, source,
language, meetily_id, claude_session_id, session_owner`) and `transcript.md` (`# Transcript: <title>`,
then lines `[hh:mm:ss] (mic|others) text`). This skill adds `summary.md` and `suggestions.md`.

## Rules for every command

- The base instructions' non-negotiables apply to every step. This skill never relaxes them.
- Work only inside the given meeting folder, plus the directories this session was given. Never
  open another project's folder under `meetings/`.
- A **yes** is the person's next reply to your question, and it says yes (`yes`, `y`, `sim`, `s`,
  or an unmistakable equivalent). Anything else is a no. Ask one yes/no question at a time.
- Never create an issue in any tracker, never send a message to another session or person, and
  never push to a branch other than the current one.

## process <folder>

1. Read `meeting.md` and all of `transcript.md`. If the file is too long for one read, read
   consecutive ranges of 800 lines until the end and tell the person "The transcript has <n>
   lines; I read it in <k> parts." Never summarize an unread part.
2. Write `<folder>/summary.md` in exactly this format:

   ```
   # Summary: <title>

   ## Summary
   <three to eight sentences>

   ## Decisions
   - <decision> — "<quoted line text>" [hh:mm:ss]
   (or the single line: No decisions were stated.)

   ## Action items
   - [ ] <action> — owner: <mic | others | a name spoken in the line | owner not stated>; due: <date as said | date not stated> — "<quoted line text>" [hh:mm:ss]
   (or the single line: No action items were stated.)
   ```

3. Show the summary to the person, then run **Save** with the message
   `meeting(<project>): <date> <title>`.
4. Whatever the save outcome, continue with **suggest** for the same folder.

## Save

Used by process and accept.

1. Run `git config core.hooksPath`. If it is not `.githooks`, stop and tell the person to run
   `git config core.hooksPath .githooks`; commit nothing.
2. Run `git diff --cached --name-only`. If any staged path is outside the meeting folder, stop and
   list those paths; commit nothing.
3. Run `git add -- <folder>`, then show `git diff --cached --stat -- <folder>` and
   `git diff --cached -- <folder>`.
4. Ask: "Commit and push these files to <current branch>? Reply yes to commit and push."
5. On a yes: run `git commit -m "<message>"`, then `git push -u origin HEAD`. If the hook rejects
   the commit, show its output and stop. If the push fails, show the error, say "The commit is
   saved locally and was not pushed", and do not retry.
6. On anything else: say "Nothing was committed. The files stay staged." and continue.

## suggest <folder>

1. Read `summary.md` and `transcript.md`.
2. Draft tasks per the base instructions ("Drafting a task") and the person's style (number and size).
3. Read every source the project file's `## Existing work` section names (and the per-project person
   file, if it adds any), using whatever tool that source says. Note each as `read` or
   `unavailable: <reason>`. Never guess an identifier.
4. Give each suggestion one verdict: `new`, `duplicate of <X>` or `add to <X>`, where X is the
   source's own identifier. When no source is named, every verdict is `new`.
5. Write `<folder>/suggestions.md` in exactly this format:

   ```
   # Suggestions: <title>

   Sources checked:
   - <source as named in the instructions>: read
   - <source>: unavailable: <reason>
   (or the single line: - none named; every verdict is `new`)

   ## S-01: <task title>

   - **Why:** <one or two sentences>
   - **Verdict:** new | duplicate of <X> | add to <X>
   - **Quotes:**
     - "<quoted line text>" [hh:mm:ss]
   - **Decision:** undecided
   - **Outcome:** —
   ```

   Number the ids `S-01`, `S-02`, … in order. Every suggestion has at least one quoted transcript
   line. Writing this file needs no yes, because it commits nothing.
6. List the suggestions to the person (id, title, verdict) and ask which to accept and which to
   reject, for example "accept S-01, reject S-02".

## accept <folder>

1. For each suggestion the person named, set **Decision** to `accepted` or `rejected`. A recorded
   decision changes only when the person explicitly asks to change it. Suggestions not named stay
   `undecided`.
2. For each `accepted` suggestion:
   - If the per-project person instructions have an `## On accept` section, follow it. Every action
     in it that writes a file outside the meeting folder needs its own yes first: show the path
     and the full content, ask, and write only on a yes. If the recipe asks to send a message or
     create an issue, print the text instead (see the rules above).
   - If there is no `## On accept` section, print the task (title, why, verdict, quotes) and do
     nothing else.
   - Set **Outcome** to what happened: the file path written and any line printed, or `printed`.
     A refused write gives `not written (no yes)`.
3. For `rejected` suggestions, **Outcome** stays `—`; do nothing else.
4. Run **Save** with the message `meeting(<project>): suggestions for <date> <title>`.
