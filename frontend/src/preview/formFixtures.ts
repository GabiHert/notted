// Preview only: the forms the form-* screens show, all on the summarized fixture meeting.
import type { wails } from "../api/bindings";
import type { Screen } from "./params";

const piece = (over: Partial<wails.FormPieceDTO>): wails.FormPieceDTO =>
  ({ kind: "text", id: "", markdown: "", html: "", question: "", options: [], placeholder: "", multiline: false, required: false, yesLabel: "Yes", noLabel: "No", ...over }) as wails.FormPieceDTO;
const opt = (label: string, description: string, recommended = false) => ({ label, description, recommended });
const form = (id: string, title: string, pieces: wails.FormPieceDTO[], over: Partial<wails.OpenFormDTO> = {}): wails.OpenFormDTO =>
  ({ id, meetingId: "fx-summarized", kind: "ask", title, pieces, queued: 0, ...over }) as wails.OpenFormDTO;

const remote = (path: string) => "https:" + "//example.invalid/" + path;
const FENCE = "```";

const choiceForm = () =>
  form("fx-form-choice", "Where should the task go?", [
    piece({
      kind: "choice",
      id: "where",
      question: "Pick the task folder",
      options: [opt("tasks/", "The project's main task folder", true), opt("tasks/backlog/", "Ideas for later"), opt("notes/", "Loose notes, not tasks")],
    }),
  ]);

export function fixtureFor(screen: Screen): wails.OpenFormDTO | null {
  switch (screen) {
    case "form-text":
      return form("fx-form-text", "Before I write the files", [
        piece({
          kind: "text",
          id: "plan",
          markdown:
            "## What happens next\n\n- I write **two** task files.\n- I show each one before I save it.\n\n| File | Size |\n| --- | --- |\n| tasks/export.md | 12 lines |\n\nSee [the guide](" +
            remote("guide") +
            ").",
        }),
      ]);
    case "form-html":
      return form("fx-form-html", "Preview of the new task", [
        piece({
          kind: "html",
          id: "preview",
          html:
            "<h3>Ship the CSV export</h3><p id=\"t\">Static text. If a script ran, this line would say so.</p><script>document.getElementById('t').textContent='A script ran.'</script><img src=\"" +
            remote("pixel.png") +
            "\" alt=\"remote image (not loaded)\">",
        }),
      ]);
    case "form-choice":
      return choiceForm();
    case "form-waiting":
      return { ...choiceForm(), id: "fx-form-waiting", queued: 2 } as wails.OpenFormDTO;
    case "form-multi":
      return form("fx-form-multi", "Review suggestions: Weekly sync", [
        piece({
          kind: "multi",
          id: "accept",
          question: "Which suggestions do you accept? The ones you leave unchecked are rejected.",
          options: [
            opt("S-01: Ship the CSV export", "new. The client asked for it twice."),
            opt("S-02: Fix the login timeout", "add to tasks/login.md. It was reported again."),
            opt("S-03: Rename the dashboard", "new. Mentioned once as an idea."),
          ],
        }),
      ]);
    case "form-input":
      return form("fx-form-input", "One detail", [
        piece({ kind: "input", id: "owner", question: "Who owns the export task?", placeholder: 'A name or "me"', required: true }),
        piece({ kind: "input", id: "why", question: "Anything to add to the task?", multiline: true }),
      ]);
    case "form-confirm":
      return form("fx-form-confirm", "Write tasks/export.md?", [
        piece({
          kind: "text",
          id: "file",
          markdown: `${FENCE}markdown\n# Ship the CSV export\n\nOwner: me\n\n- [ ] Add the export button\n- [ ] Write the CSV file\n${FENCE}`,
        }),
        piece({ kind: "confirm", id: "go", question: "Write this file?", yesLabel: "Write it", noLabel: "Not this one" }),
      ]);
    case "form-permission":
      return form(
        "fx-form-permission",
        "Claude Code wants to use Bash",
        [
          piece({
            kind: "text",
            id: "request",
            markdown: `${FENCE}json\n{\n  "command": "git commit -m \\"meeting(demo): suggestions for 2026-10-25 Weekly sync\\""\n}\n${FENCE}`,
          }),
          piece({ kind: "confirm", id: "allow", question: "Allow this?", yesLabel: "Allow", noLabel: "Deny" }),
        ],
        { kind: "permission" },
      );
    default:
      return null;
  }
}
