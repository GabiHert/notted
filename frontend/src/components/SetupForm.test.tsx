import { useState } from "react";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { SetupForm } from "./SetupForm";
import { ChooseFolder, CloneArchive, InspectArchive, PrepareArchive, CreateMyProjectFile, CreateProfile, CreateProject, DeleteProfile, ListProfileFiles, SetSettings, GetProjectDeletePlan, DeleteProject, CommitProjectDeletion, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

const SAVED = { profile: "ana", checkoutPath: "/archive", projectRoots: { acme: "/work/acme" } };

beforeEach(resetBindings);

function setup(over: Record<string, unknown> = {}) {
  const props = {
    saved: SAVED,
    settingsError: "",
    projects: ["acme", "globex"],
    projectsError: "",
    profiles: ["ana", "bruno"],
    profilesError: "",
    onSaved: vi.fn(),
    onOpen: vi.fn(),
    onDeleted: vi.fn(),
    ...over,
  };
  const view = render(<SetupForm {...(props as any)} />);
  return { ...props, ...view };
}

const type = (label: string | RegExp, value: string) => fireEvent.change(screen.getByLabelText(label), { target: { value } });
const click = (name: string | RegExp) => fireEvent.click(screen.getByRole("button", { name }));
const deferred = <T,>() => {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((a, b) => ((resolve = a), (reject = b)));
  return { promise, resolve, reject };
};

describe("fields and typed save", () => {
  test("fills the fields from the saved settings and shows the help texts", () => {
    setup();
    expect(screen.getByText("Your setup")).toBeInTheDocument();
    expect(screen.getByLabelText("Archive folder")).toHaveValue("/archive");
    expect(screen.getByLabelText("acme")).toHaveValue("/work/acme");
    expect(screen.getByLabelText("globex")).toHaveValue("");
    expect(screen.getByLabelText("globex")).toHaveAttribute("placeholder", "/full/path/to/the/project");
    expect(screen.getByText("The full path of your meetings archive folder. Any folder works: Notted sets it up.")).toBeInTheDocument();
    expect(screen.getByText("Projects")).toBeInTheDocument();
    expect(screen.getByText("Optional. A folder per project that Claude Code may read during that project's meetings.")).toBeInTheDocument();
    expect(screen.getByText("Your file in instructions/people/ of the archive.")).toBeInTheDocument();
  });

  test("Save settings sends the typed archive and roots with the saved profile, dropping empty roots", async () => {
    const { onSaved } = setup();
    type("Archive folder", " /other ");
    type("globex", "/work/globex");
    type("acme", "");
    click("Save settings");
    expect(await screen.findByText("Settings saved.")).toBeInTheDocument();
    expect(SetSettings).toHaveBeenCalledWith({ profile: "ana", checkoutPath: "/other", projectRoots: { globex: "/work/globex" } });
    expect(onSaved).toHaveBeenCalledTimes(1);
  });

  test("Settings saved. fades after 3 seconds", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      setup();
      click("Save settings");
      expect(await screen.findByText("Settings saved.")).toBeInTheDocument();
      act(() => {
        vi.advanceTimersByTime(3100);
      });
      expect(screen.queryByText("Settings saved.")).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  test("a rejected save is shown verbatim in an alert and no saved message appears", async () => {
    SetSettings.mockRejectedValue(new Error("The archive folder does not exist: /nope"));
    setup();
    click("Save settings");
    expect(await screen.findByRole("alert")).toHaveTextContent("The archive folder does not exist: /nope");
    expect(screen.queryByText("Settings saved.")).not.toBeInTheDocument();
  });

  test("a settings error is shown in the error card", () => {
    setup({ saved: null, settingsError: "Could not read the settings" });
    expect(screen.getByRole("alert")).toHaveTextContent("Could not read the settings");
  });

  test("Save settings is disabled while saving, so a double click saves once", async () => {
    const d = deferred<void>();
    SetSettings.mockReturnValue(d.promise);
    setup();
    click("Save settings");
    expect(screen.getByRole("button", { name: "Save settings" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
    await act(async () => d.resolve());
    expect(SetSettings).toHaveBeenCalledTimes(1);
  });

  test("a save that resolves after unmount does not call onSaved", async () => {
    const d = deferred<void>();
    SetSettings.mockReturnValue(d.promise);
    const { onSaved, unmount } = setup();
    click("Save settings");
    unmount();
    await act(async () => d.resolve());
    expect(onSaved).not.toHaveBeenCalled();
  });
});

describe("archive folder picker", () => {
  test("Choose… calls ChooseFolder with the title and the field, and a path saves at once", async () => {
    ChooseFolder.mockResolvedValue("/picked");
    const { onSaved } = setup();
    type("Archive folder", "/typed");
    click("Choose the meetings archive folder");
    expect(screen.getAllByRole("button", { name: "Choose the meetings archive folder" })[0]).toHaveTextContent("Choose…");
    expect(await screen.findByText("Settings saved.")).toBeInTheDocument();
    expect(ChooseFolder).toHaveBeenCalledWith("Choose the meetings archive folder", "/typed");
    expect(SetSettings).toHaveBeenCalledWith({ profile: "ana", checkoutPath: "/picked", projectRoots: { acme: "/work/acme" } });
    expect(screen.getByLabelText("Archive folder")).toHaveValue("/picked");
    expect(onSaved).toHaveBeenCalledTimes(1);
  });

  test("a saved pick ignores unsaved typed text in other fields", async () => {
    ChooseFolder.mockResolvedValue("/picked");
    setup();
    type("globex", "/typed/globex");
    click("Choose the meetings archive folder");
    await screen.findByText("Settings saved.");
    expect(SetSettings).toHaveBeenCalledWith({ profile: "ana", checkoutPath: "/picked", projectRoots: { acme: "/work/acme" } });
  });

  test("a cancelled picker changes nothing and saves nothing", async () => {
    ChooseFolder.mockResolvedValue("");
    const { onSaved } = setup();
    click("Choose the meetings archive folder");
    await act(async () => {});
    expect(SetSettings).not.toHaveBeenCalled();
    expect(onSaved).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Archive folder")).toHaveValue("/archive");
  });

  test("a rejected save keeps the picked path in the field and shows the message verbatim", async () => {
    ChooseFolder.mockResolvedValue("/picked");
    SetSettings.mockRejectedValue(new Error("That is not a git checkout: /picked"));
    setup();
    click("Choose the meetings archive folder");
    expect(await screen.findByRole("alert")).toHaveTextContent("That is not a git checkout: /picked");
    expect(screen.getByLabelText("Archive folder")).toHaveValue("/picked");
  });

  test("a rejected picker shows its message", async () => {
    ChooseFolder.mockRejectedValue(new Error("The folder dialog failed"));
    setup();
    click("Choose the meetings archive folder");
    expect(await screen.findByRole("alert")).toHaveTextContent("The folder dialog failed");
  });

  test("the button is disabled while the picker is open", async () => {
    const d = deferred<string>();
    ChooseFolder.mockReturnValue(d.promise);
    setup();
    click("Choose the meetings archive folder");
    expect(screen.getByRole("button", { name: "Choose the meetings archive folder" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Choose the meetings archive folder" }));
    await act(async () => d.resolve(""));
    expect(ChooseFolder).toHaveBeenCalledTimes(1);
  });

  test("a picker that resolves after unmount saves nothing", async () => {
    const d = deferred<string>();
    ChooseFolder.mockReturnValue(d.promise);
    const { unmount, onSaved } = setup();
    click("Choose the meetings archive folder");
    unmount();
    await act(async () => d.resolve("/late"));
    expect(SetSettings).not.toHaveBeenCalled();
    expect(onSaved).not.toHaveBeenCalled();
  });
});

const NEEDS = ["bin/meeting", "prompts/base.md", ".gitignore (adds the recording rules)"];
const check = (over: Record<string, unknown> = {}) => ({ path: "/picked", missing: NEEDS, inRepo: false, nested: false, repoRoot: "", ready: false, ...over });
const GIT_INIT_LINE = "This folder is not a git repository yet. Notted runs git init in it so you can commit meetings. It has no remote, so meetings are committed on this Mac only until you add one.";

describe("a folder that is not set up as an archive yet", () => {
  test("a ready folder is inspected and saved with no panel", async () => {
    ChooseFolder.mockResolvedValue("/picked");
    setup();
    click("Choose the meetings archive folder");
    await screen.findByText("Settings saved.");
    expect(InspectArchive).toHaveBeenCalledWith("/picked");
    expect(screen.queryByRole("region", { name: "Set up this folder as your meetings archive?" })).not.toBeInTheDocument();
    expect(PrepareArchive).not.toHaveBeenCalled();
  });

  test("a folder outside any repository shows the panel with the list and the git init line, and saves nothing", async () => {
    ChooseFolder.mockResolvedValue("/picked");
    InspectArchive.mockResolvedValue(check());
    setup();
    click("Choose the meetings archive folder");
    const region = await screen.findByRole("region", { name: "Set up this folder as your meetings archive?" });
    const heading = within(region).getByRole("heading", { name: "Set up this folder as your meetings archive?" });
    expect(heading).toHaveFocus();
    expect(within(region).getByText("Notted adds these files to /picked. It does not change or remove anything already there.")).toBeInTheDocument();
    for (const f of NEEDS) expect(within(region).getByText(f).tagName).toBe("CODE");
    expect(within(region).getByText(GIT_INIT_LINE)).toBeInTheDocument();
    expect(within(region).queryByText(/is inside the git repository/)).not.toBeInTheDocument();
    expect(SetSettings).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Archive folder")).toHaveValue("/archive");
  });

  test("a folder inside another repository shows the repository line and no git init line", async () => {
    ChooseFolder.mockResolvedValue("/picked");
    InspectArchive.mockResolvedValue(check({ inRepo: true, nested: true, repoRoot: "/home/ana/notes" }));
    setup();
    click("Choose the meetings archive folder");
    const region = await screen.findByRole("region", { name: "Set up this folder as your meetings archive?" });
    expect(within(region).getByText("This folder is inside the git repository at /home/ana/notes. Meetings are committed there.")).toBeInTheDocument();
    expect(within(region).queryByText(GIT_INIT_LINE)).not.toBeInTheDocument();
  });

  test("the top of a repository shows neither line", async () => {
    ChooseFolder.mockResolvedValue("/picked");
    InspectArchive.mockResolvedValue(check({ inRepo: true, repoRoot: "/picked" }));
    setup();
    click("Choose the meetings archive folder");
    const region = await screen.findByRole("region", { name: "Set up this folder as your meetings archive?" });
    expect(within(region).queryByText(/git repository/)).not.toBeInTheDocument();
  });

  test("a repository top whose path differs from git's spelling shows neither line", async () => {
    ChooseFolder.mockResolvedValue("/picked/");
    InspectArchive.mockResolvedValue(check({ path: "/picked/", inRepo: true, nested: false, repoRoot: "/private/picked" }));
    setup();
    click("Choose the meetings archive folder");
    const region = await screen.findByRole("region", { name: "Set up this folder as your meetings archive?" });
    expect(within(region).queryByText(/git repository/)).not.toBeInTheDocument();
  });

  test("Set up this folder prepares it, then saves the setting", async () => {
    ChooseFolder.mockResolvedValue("/picked");
    InspectArchive.mockResolvedValue(check());
    const { onSaved } = setup();
    click("Choose the meetings archive folder");
    await screen.findByRole("region", { name: "Set up this folder as your meetings archive?" });
    click("Set up this folder");
    expect(await screen.findByText("Settings saved.")).toBeInTheDocument();
    expect(PrepareArchive).toHaveBeenCalledWith("/picked");
    expect(SetSettings).toHaveBeenCalledWith({ profile: "ana", checkoutPath: "/picked", projectRoots: { acme: "/work/acme" } });
    expect(PrepareArchive.mock.invocationCallOrder[0]).toBeLessThan(SetSettings.mock.invocationCallOrder[0]);
    expect(screen.queryByRole("region", { name: "Set up this folder as your meetings archive?" })).not.toBeInTheDocument();
    expect(screen.getByLabelText("Archive folder")).toHaveValue("/picked");
    expect(screen.getByLabelText("Archive folder")).toHaveFocus();
    expect(onSaved).toHaveBeenCalledTimes(1);
  });

  test("Choose another folder closes the panel and saves nothing", async () => {
    ChooseFolder.mockResolvedValue("/picked");
    InspectArchive.mockResolvedValue(check());
    setup();
    click("Choose the meetings archive folder");
    await screen.findByRole("region", { name: "Set up this folder as your meetings archive?" });
    click("Choose another folder");
    expect(screen.queryByRole("region", { name: "Set up this folder as your meetings archive?" })).not.toBeInTheDocument();
    expect(PrepareArchive).not.toHaveBeenCalled();
    expect(SetSettings).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Archive folder")).toHaveValue("/archive");
  });

  test("a PrepareArchive error shows in the form's error line and saves nothing", async () => {
    ChooseFolder.mockResolvedValue("/picked");
    InspectArchive.mockResolvedValue(check());
    PrepareArchive.mockRejectedValue(new Error("Could not set up /picked: permission denied"));
    setup();
    click("Choose the meetings archive folder");
    await screen.findByRole("region", { name: "Set up this folder as your meetings archive?" });
    click("Set up this folder");
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not set up /picked: permission denied");
    expect(SetSettings).not.toHaveBeenCalled();
  });

  test("an InspectArchive error shows in the error line", async () => {
    ChooseFolder.mockResolvedValue("/picked");
    InspectArchive.mockRejectedValue(new Error("That folder does not exist: /picked"));
    setup();
    click("Choose the meetings archive folder");
    expect(await screen.findByRole("alert")).toHaveTextContent("That folder does not exist: /picked");
    expect(SetSettings).not.toHaveBeenCalled();
  });

  test("a typed path is inspected on save, and the panel's setup saves the typed settings", async () => {
    InspectArchive.mockResolvedValue(check({ path: "/typed" }));
    setup();
    type("Archive folder", " /typed ");
    type("globex", "/work/globex");
    click("Save settings");
    await screen.findByRole("region", { name: "Set up this folder as your meetings archive?" });
    expect(InspectArchive).toHaveBeenCalledWith("/typed");
    expect(SetSettings).not.toHaveBeenCalled();
    click("Set up this folder");
    await screen.findByText("Settings saved.");
    expect(PrepareArchive).toHaveBeenCalledWith("/typed");
    expect(SetSettings).toHaveBeenCalledWith({ profile: "ana", checkoutPath: "/typed", projectRoots: { acme: "/work/acme", globex: "/work/globex" } });
  });

  test("clearing the archive field saves without inspecting", async () => {
    setup();
    type("Archive folder", "");
    click("Save settings");
    await screen.findByText("Settings saved.");
    expect(InspectArchive).not.toHaveBeenCalled();
  });
});

describe("clone panel", () => {
  const open = () => click("Clone the archive from GitHub instead");

  test("the link opens the panel and hides itself; Cancel closes it and the link returns", () => {
    setup();
    open();
    expect(screen.queryByRole("button", { name: "Clone the archive from GitHub instead" })).not.toBeInTheDocument();
    expect(screen.getByLabelText("GitHub address")).toHaveFocus();
    expect(screen.getByText("The address you would give to git clone, from the Code button on the archive's GitHub page. Notted runs your own git with your own login.")).toBeInTheDocument();
    expect(screen.getByText("Notted creates the archive folder inside this folder.")).toBeInTheDocument();
    click("Cancel");
    expect(screen.queryByLabelText("GitHub address")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Clone the archive from GitHub instead" })).toHaveFocus();
  });

  test("Clone archive is disabled until both fields are filled", () => {
    setup();
    open();
    expect(screen.getByRole("button", { name: "Clone archive" })).toBeDisabled();
    type("GitHub address", "git@example.invalid:acme/meetings-archive.git");
    expect(screen.getByRole("button", { name: "Clone archive" })).toBeDisabled();
    type("Clone into", "/x");
    expect(screen.getByRole("button", { name: "Clone archive" })).toBeEnabled();
  });

  test("the destination Choose… fills the field and saves nothing", async () => {
    ChooseFolder.mockResolvedValue("/Users/ana/code");
    setup();
    open();
    click("Choose where to clone the archive");
    await act(async () => {});
    expect(ChooseFolder).toHaveBeenCalledWith("Choose where to clone the archive", "");
    expect(screen.getByLabelText("Clone into")).toHaveValue("/Users/ana/code");
    expect(SetSettings).not.toHaveBeenCalled();
  });

  test("shows Cloning… while it runs, disables Clone and Cancel, and runs once", async () => {
    const d = deferred<string>();
    CloneArchive.mockReturnValue(d.promise);
    setup();
    open();
    type("GitHub address", "addr");
    type("Clone into", "/x");
    click("Clone archive");
    const busy = screen.getByRole("button", { name: "Cloning…" });
    expect(busy).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    fireEvent.click(busy);
    await act(async () => d.reject(new Error("stop")));
    expect(CloneArchive).toHaveBeenCalledTimes(1);
  });

  test("success saves the archive, closes the panel and shows the notice", async () => {
    const { onSaved } = setup();
    open();
    type("GitHub address", "addr");
    type("Clone into", "/x");
    click("Clone archive");
    expect(await screen.findByText("Cloned into /x/meetings-archive. It is now your archive folder.")).toBeInTheDocument();
    expect(CloneArchive).toHaveBeenCalledWith("addr", "/x");
    expect(SetSettings).toHaveBeenCalledWith({ profile: "ana", checkoutPath: "/x/meetings-archive", projectRoots: { acme: "/work/acme" } });
    expect(screen.getByLabelText("Archive folder")).toHaveValue("/x/meetings-archive");
    expect(screen.queryByLabelText("GitHub address")).not.toBeInTheDocument();
    expect(onSaved).toHaveBeenCalledTimes(1);
  });

  test("a rejection is shown verbatim inside the panel and the panel stays open", async () => {
    CloneArchive.mockRejectedValue(new Error("git clone failed:\nERROR: Repository not found."));
    setup();
    open();
    type("GitHub address", "addr");
    type("Clone into", "/x");
    click("Clone archive");
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("git clone failed:");
    expect(alert).toHaveTextContent("ERROR: Repository not found.");
    expect(within(alert.closest(".nt-setup__inline") as HTMLElement).getByLabelText("GitHub address")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Clone archive" })).toBeEnabled();
    expect(SetSettings).not.toHaveBeenCalled();
  });

  test("a clone that resolves after unmount saves nothing", async () => {
    const d = deferred<string>();
    CloneArchive.mockReturnValue(d.promise);
    const { unmount, onSaved } = setup();
    open();
    type("GitHub address", "addr");
    type("Clone into", "/x");
    click("Clone archive");
    unmount();
    await act(async () => d.resolve("/x/meetings-archive"));
    expect(SetSettings).not.toHaveBeenCalled();
    expect(onSaved).not.toHaveBeenCalled();
  });

  test("a destination picker that resolves after Cancel does not fill a closed panel", async () => {
    const d = deferred<string>();
    ChooseFolder.mockReturnValue(d.promise);
    setup();
    open();
    click("Choose where to clone the archive");
    await act(async () => {});
    click("Cancel");
    await act(async () => d.resolve("/late"));
    open();
    expect(screen.getByLabelText("Clone into")).toHaveValue("");
  });
});

describe("profile", () => {
  test("is disabled with a help line when there is no archive", () => {
    setup({ saved: { profile: "", checkoutPath: "", projectRoots: {} }, profiles: [], projects: [] });
    expect(screen.getByLabelText("Profile")).toBeDisabled();
    expect(screen.getAllByText("Choose the archive folder first.")).toHaveLength(2);
  });

  test("lists the placeholder, the names and New profile…, with the saved profile selected", () => {
    setup();
    const options = within(screen.getByLabelText("Profile")).getAllByRole("option").map((o) => o.textContent);
    expect(options).toEqual(["Choose your profile", "ana", "bruno", "New profile…"]);
    expect(screen.getByLabelText("Profile")).toHaveValue("ana");
    expect(screen.getByRole("option", { name: "Choose your profile" })).toBeDisabled();
  });

  test("keeps a saved profile that is missing from the list", () => {
    setup({ saved: { ...SAVED, profile: "zed" } });
    expect(screen.getByRole("option", { name: "zed" })).toBeInTheDocument();
    expect(screen.getByLabelText("Profile")).toHaveValue("zed");
  });

  test("an unset profile shows the placeholder", () => {
    setup({ saved: { ...SAVED, profile: "" } });
    expect(screen.getByLabelText("Profile")).toHaveValue("");
  });

  test("shows the profiles error verbatim", () => {
    setup({ profilesError: "Could not read instructions/people" });
    expect(screen.getByText("Could not read instructions/people")).toBeInTheDocument();
  });

  test("choosing a name saves it at once with the saved archive and roots", async () => {
    const { onSaved } = setup();
    fireEvent.change(screen.getByLabelText("Profile"), { target: { value: "bruno" } });
    expect(await screen.findByText("Settings saved.")).toBeInTheDocument();
    expect(SetSettings).toHaveBeenCalledWith({ profile: "bruno", checkoutPath: "/archive", projectRoots: { acme: "/work/acme" } });
    expect(onSaved).toHaveBeenCalledTimes(1);
  });

  test("New profile… opens the form without saving and leaves the select on its value", () => {
    setup();
    fireEvent.change(screen.getByLabelText("Profile"), { target: { value: "__new__" } });
    expect(SetSettings).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Profile")).toHaveValue("ana");
    expect(screen.getByLabelText("New profile name")).toHaveFocus();
    expect(screen.getByText("Use lowercase letters, digits and dashes, for example ana or ana-lima.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create profile" })).toBeDisabled();
  });

  test("Create profile creates, saves the profile, closes the form, shows the notice and opens the file", async () => {
    const { onOpen, onSaved } = setup();
    fireEvent.change(screen.getByLabelText("Profile"), { target: { value: "__new__" } });
    type("New profile name", "carla");
    click("Create profile");
    expect(await screen.findByText("Created instructions/people/carla.md. Fill it in below.")).toBeInTheDocument();
    expect(CreateProfile).toHaveBeenCalledWith("carla");
    expect(SetSettings).toHaveBeenCalledWith({ profile: "carla", checkoutPath: "/archive", projectRoots: { acme: "/work/acme" } });
    expect(onOpen).toHaveBeenCalledWith("instructions/people/carla.md");
    expect(onSaved).toHaveBeenCalledTimes(1);
    expect(screen.queryByLabelText("New profile name")).not.toBeInTheDocument();
  });

  test("a CreateProfile rejection stays in the form", async () => {
    CreateProfile.mockRejectedValue(new Error("instructions/people/carla.md already exists"));
    const { onOpen } = setup();
    fireEvent.change(screen.getByLabelText("Profile"), { target: { value: "__new__" } });
    type("New profile name", "carla");
    click("Create profile");
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("instructions/people/carla.md already exists");
    expect(alert.closest(".nt-setup__inline")).not.toBeNull();
    expect(screen.getByLabelText("New profile name")).toBeInTheDocument();
    expect(SetSettings).not.toHaveBeenCalled();
    expect(onOpen).not.toHaveBeenCalled();
  });

  test("a SetSettings rejection after creating also stays in the form", async () => {
    SetSettings.mockRejectedValue(new Error("Could not save the profile"));
    setup();
    fireEvent.change(screen.getByLabelText("Profile"), { target: { value: "__new__" } });
    type("New profile name", "carla");
    click("Create profile");
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not save the profile");
    expect(screen.getByLabelText("New profile name")).toBeInTheDocument();
  });

  test("Create profile runs once on a double click and Cancel closes the form", async () => {
    const d = deferred<string>();
    CreateProfile.mockReturnValue(d.promise);
    setup();
    fireEvent.change(screen.getByLabelText("Profile"), { target: { value: "__new__" } });
    type("New profile name", "carla");
    click("Create profile");
    expect(screen.getByRole("button", { name: "Create profile" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Create profile" }));
    await act(async () => d.reject(new Error("x")));
    expect(CreateProfile).toHaveBeenCalledTimes(1);
    click("Cancel");
    expect(screen.queryByLabelText("New profile name")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Profile")).toHaveFocus();
  });

  test("a profile created after unmount does not save or open", async () => {
    const d = deferred<string>();
    CreateProfile.mockReturnValue(d.promise);
    const { unmount, onOpen } = setup();
    fireEvent.change(screen.getByLabelText("Profile"), { target: { value: "__new__" } });
    type("New profile name", "carla");
    click("Create profile");
    unmount();
    await act(async () => d.resolve("instructions/people/carla.md"));
    expect(SetSettings).not.toHaveBeenCalled();
    expect(onOpen).not.toHaveBeenCalled();
  });
});

describe("projects", () => {
  test("says Choose the archive folder first. and shows no rows without an archive", () => {
    setup({ saved: { profile: "ana", checkoutPath: "", projectRoots: {} }, projects: ["acme"] });
    expect(screen.queryByLabelText("acme")).not.toBeInTheDocument();
    expect(screen.getAllByText("Choose the archive folder first.")).toHaveLength(2);
  });

  test("says No projects yet. Add one with New project. when the list is empty", () => {
    setup({ projects: [] });
    expect(screen.getByText("No projects yet. Add one with New project.")).toBeInTheDocument();
  });

  test("shows the projects error verbatim", () => {
    setup({ projects: [], projectsError: "Could not list the projects" });
    expect(screen.getByText("Could not list the projects")).toBeInTheDocument();
  });

  test("a row's Choose… passes the title and field and saves the merged roots at once", async () => {
    ChooseFolder.mockResolvedValue("/picked/globex");
    setup();
    click("Choose the root folder for globex");
    expect(await screen.findByText("Settings saved.")).toBeInTheDocument();
    expect(ChooseFolder).toHaveBeenCalledWith("Choose the root folder for globex", "");
    expect(SetSettings).toHaveBeenCalledWith({ profile: "ana", checkoutPath: "/archive", projectRoots: { acme: "/work/acme", globex: "/picked/globex" } });
    expect(screen.getByLabelText("globex")).toHaveValue("/picked/globex");
  });

  test("a cancelled row picker saves nothing", async () => {
    setup();
    click("Choose the root folder for acme");
    await act(async () => {});
    expect(ChooseFolder).toHaveBeenCalledWith("Choose the root folder for acme", "/work/acme");
    expect(SetSettings).not.toHaveBeenCalled();
  });

  test("New project… is disabled with Choose your profile first. when no profile", () => {
    setup({ saved: { ...SAVED, profile: "" } });
    expect(screen.getByRole("button", { name: "New project…" })).toBeDisabled();
    expect(screen.getByText("Choose your profile first.")).toBeInTheDocument();
  });

  test("creating a project shows the two-path notice, reloads and opens the first path", async () => {
    const { onOpen, onSaved } = setup();
    click("New project…");
    expect(screen.getByLabelText("New project key")).toHaveFocus();
    expect(screen.getByText("Use lowercase letters, digits and dashes, for example acme or acme-web. Notted creates the project's file and your own file for it.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create project" })).toBeDisabled();
    type("New project key", "initech-web");
    click("Create project");
    expect(await screen.findByText("Created instructions/projects/initech-web.md and instructions/people/ana/initech-web.md. Fill them in below.")).toBeInTheDocument();
    expect(CreateProject).toHaveBeenCalledWith("initech-web");
    expect(onOpen).toHaveBeenCalledWith("instructions/projects/initech-web.md");
    expect(onSaved).toHaveBeenCalledTimes(1);
    expect(screen.queryByLabelText("New project key")).not.toBeInTheDocument();
  });

  test("one returned path uses the one-path notice", async () => {
    CreateProject.mockResolvedValue(["instructions/projects/initech-web.md"]);
    setup();
    click("New project…");
    type("New project key", "initech-web");
    click("Create project");
    expect(await screen.findByText("Created instructions/projects/initech-web.md. Fill it in below.")).toBeInTheDocument();
  });

  test("a rejection stays in the form and Cancel closes it", async () => {
    CreateProject.mockRejectedValue(new Error("instructions/projects/acme.md already exists"));
    const { onOpen } = setup();
    click("New project…");
    type("New project key", "acme");
    click("Create project");
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("instructions/projects/acme.md already exists");
    expect(alert.closest(".nt-setup__inline")).not.toBeNull();
    expect(onOpen).not.toHaveBeenCalled();
    click("Cancel");
    expect(screen.queryByLabelText("New project key")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "New project…" })).toHaveFocus();
  });

  test("Create project runs once on a double click", async () => {
    const d = deferred<string[]>();
    CreateProject.mockReturnValue(d.promise);
    setup();
    click("New project…");
    type("New project key", "initech-web");
    click("Create project");
    fireEvent.click(screen.getByRole("button", { name: "Create project" }));
    await act(async () => d.reject(new Error("x")));
    expect(CreateProject).toHaveBeenCalledTimes(1);
  });

  test("a project created after unmount does not open or reload", async () => {
    const d = deferred<string[]>();
    CreateProject.mockReturnValue(d.promise);
    const { unmount, onOpen, onSaved } = setup();
    click("New project…");
    type("New project key", "initech-web");
    click("Create project");
    unmount();
    await act(async () => d.resolve(["instructions/projects/initech-web.md"]));
    expect(onOpen).not.toHaveBeenCalled();
    expect(onSaved).not.toHaveBeenCalled();
  });
});

// Like SettingsView: onSaved hands the form a fresh `saved` object, as a reload does.
function Live({ onOpen }: { onOpen: (p: string) => void }) {
  const [saved, setSaved] = useState(SAVED);
  const reload = () => {
    const last = SetSettings.mock.calls.at(-1)?.[0];
    setSaved({ ...(last ?? saved), projectRoots: { ...((last ?? saved).projectRoots ?? {}) } });
  };
  return <SetupForm saved={saved} settingsError="" projects={["acme", "globex"]} projectsError="" profiles={["ana", "bruno"]} profilesError="" onSaved={reload} onOpen={onOpen} />;
}

describe("a reload keeps unsaved typed text", () => {
  test("text typed in a project root survives a pick in the archive field", async () => {
    ChooseFolder.mockResolvedValue("/picked");
    render(<Live onOpen={() => {}} />);
    type("globex", "/typed/globex");
    click("Choose the meetings archive folder");
    await screen.findByText("Settings saved.");
    expect(screen.getByLabelText("Archive folder")).toHaveValue("/picked");
    expect(screen.getByLabelText("globex")).toHaveValue("/typed/globex");
    expect(screen.getByLabelText("acme")).toHaveValue("/work/acme");
  });

  test("text typed in the archive field survives a root pick, and the picked root takes its value", async () => {
    ChooseFolder.mockResolvedValue("/picked/globex");
    render(<Live onOpen={() => {}} />);
    type("Archive folder", "/typed/archive");
    click("Choose the root folder for globex");
    await screen.findByText("Settings saved.");
    expect(screen.getByLabelText("Archive folder")).toHaveValue("/typed/archive");
    expect(screen.getByLabelText("globex")).toHaveValue("/picked/globex");
  });

  test("text typed in a project root survives New project", async () => {
    const onOpen = vi.fn();
    render(<Live onOpen={onOpen} />);
    type("globex", "/typed/globex");
    click("New project…");
    type("New project key", "initech-web");
    click("Create project");
    await screen.findByText(/^Created instructions\/projects\/initech-web\.md/);
    expect(screen.getByLabelText("globex")).toHaveValue("/typed/globex");
  });

  test("a typed archive saved with Save settings leaves the field showing the saved value", async () => {
    render(<Live onOpen={() => {}} />);
    type("Archive folder", " /other ");
    click("Save settings");
    await screen.findByText("Settings saved.");
    expect(screen.getByLabelText("Archive folder")).toHaveValue("/other");
  });
});

describe("failed saves keep what the person picked", () => {
  test("a clone that succeeds but whose save is rejected shows the error in the panel and keeps the cloned path", async () => {
    SetSettings.mockRejectedValue(new Error("Could not save the archive folder"));
    setup();
    click("Clone the archive from GitHub instead");
    type("GitHub address", "addr");
    type("Clone into", "/x");
    click("Clone archive");
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Could not save the archive folder");
    expect(alert.closest(".nt-setup__inline")).not.toBeNull();
    expect(screen.getByLabelText("GitHub address")).toBeInTheDocument();
    expect(screen.getByLabelText("Archive folder")).toHaveValue("/x/meetings-archive");
  });

  test("a rejected profile save keeps the picked value in the select and shows an alert", async () => {
    SetSettings.mockRejectedValue(new Error("Could not save the profile"));
    setup();
    fireEvent.change(screen.getByLabelText("Profile"), { target: { value: "bruno" } });
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not save the profile");
    expect(screen.getByLabelText("Profile")).toHaveValue("bruno");
  });

  test("after a rejected pick, picking the saved profile again shows it and saves it", async () => {
    SetSettings.mockRejectedValueOnce(new Error("Could not save the profile"));
    render(<Live onOpen={() => {}} />);
    fireEvent.change(screen.getByLabelText("Profile"), { target: { value: "bruno" } });
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not save the profile");
    fireEvent.change(screen.getByLabelText("Profile"), { target: { value: "ana" } });
    expect(screen.getByLabelText("Profile")).toHaveValue("ana");
    await screen.findByText("Settings saved.");
    expect(SetSettings).toHaveBeenLastCalledWith({ profile: "ana", checkoutPath: "/archive", projectRoots: { acme: "/work/acme" } });
    expect(screen.getByLabelText("Profile")).toHaveValue("ana");
  });
});

const BRUNO_FILES = ["instructions/people/bruno.md", "instructions/people/bruno/", "instructions/people/bruno/acme.md"];

describe("delete a profile", () => {
  const openPanel = () => click("Delete a profile…");

  test("the link is absent without an archive and with no profiles", () => {
    const a = setup({ saved: { ...SAVED, checkoutPath: "" } });
    expect(screen.queryByRole("button", { name: "Delete a profile…" })).not.toBeInTheDocument();
    a.unmount();
    setup({ profiles: [] });
    expect(screen.queryByRole("button", { name: "Delete a profile…" })).not.toBeInTheDocument();
  });

  test("opens the panel with the sub-heading, help and one row per profile", () => {
    setup();
    openPanel();
    expect(screen.queryByRole("button", { name: "Delete a profile…" })).not.toBeInTheDocument();
    expect(screen.getByText("Delete a profile")).toBeInTheDocument();
    expect(screen.getByText("Deleting a profile removes that person's files from your archive folder. Notted does not commit the deletion.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete profile ana" })).toBeDisabled();
    expect(screen.getByText("In use. Choose another profile above first.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete profile bruno" })).toBeEnabled();
    expect(document.querySelectorAll(".nt-setup__person")).toHaveLength(2);
  });

  test("Delete… lists the files in the confirm; Keep profile closes it and deletes nothing", async () => {
    setup();
    openPanel();
    click("Delete profile bruno");
    const dialog = await screen.findByRole("dialog", { name: "Delete profile bruno?" });
    expect(ListProfileFiles).toHaveBeenCalledWith("bruno");
    expect(within(dialog).getByText("Notted deletes these files and folders from your archive folder:")).toBeInTheDocument();
    expect(within(dialog).getAllByRole("listitem").map((li) => li.textContent)).toEqual(BRUNO_FILES);
    expect(within(dialog).getByText("Nothing is committed. The deletion shows in the archive's working tree, like an edit.")).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Keep profile" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(DeleteProfile).not.toHaveBeenCalled();
  });

  test("Delete profile sends the same list, closes the panel, shows the notice and calls onDeleted", async () => {
    const { onDeleted } = setup();
    openPanel();
    click("Delete profile bruno");
    await screen.findByRole("dialog");
    click("Delete profile");
    expect(await screen.findByText("Deleted profile bruno from the archive folder.")).toBeInTheDocument();
    expect(DeleteProfile).toHaveBeenCalledWith("bruno", BRUNO_FILES);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.queryByText("Deleting a profile removes that person's files from your archive folder. Notted does not commit the deletion.")).not.toBeInTheDocument();
    expect(onDeleted).toHaveBeenCalledTimes(1);
    expect(SetSettings).not.toHaveBeenCalled();
  });

  test("a ListProfileFiles rejection shows verbatim in the panel and opens no dialog", async () => {
    ListProfileFiles.mockRejectedValue(new Error("bruno is the profile in use."));
    setup();
    openPanel();
    click("Delete profile bruno");
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("bruno is the profile in use.");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  test("a DeleteProfile rejection shows verbatim, keeps the panel open and still calls onDeleted; Close closes it", async () => {
    DeleteProfile.mockRejectedValue(new Error("Deleted 1 of 3 files, then failed: permission denied"));
    const { onDeleted } = setup();
    openPanel();
    click("Delete profile bruno");
    await screen.findByRole("dialog");
    click("Delete profile");
    expect(await screen.findByRole("alert")).toHaveTextContent("Deleted 1 of 3 files, then failed: permission denied");
    expect(onDeleted).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Delete profile bruno" })).toBeInTheDocument();
    click("Close");
    expect(screen.queryByRole("button", { name: "Delete profile bruno" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete a profile…" })).toHaveFocus();
  });
});

describe("delete a project", () => {
  const PLAN = (key: string, over: Record<string, unknown> = {}) => ({
    key,
    files: [`instructions/projects/${key}.md`, `instructions/people/ana/${key}.md`, `meetings/${key}/2026/2026-10-01-sync/meeting.md`],
    folders: [`meetings/${key}/2026/2026-10-01-sync/`, `meetings/${key}/2026/`, `meetings/${key}/`],
    localMeetings: 2,
    unsavedMeetings: 1,
    recording: false,
    ...over,
  });
  const TRACKED = ["instructions/projects/acme.md", "meetings/acme/2026/2026-10-01-sync/meeting.md"];
  const openPanel = async () => {
    GetProjectDeletePlan.mockClear();
    click("Delete a project…");
    await waitFor(() => expect(GetProjectDeletePlan).toHaveBeenCalledTimes(2));
  };
  const askAcme = async () => {
    await openPanel();
    click("Delete project acme");
    return screen.findByRole("dialog", { name: "Delete project acme?" });
  };
  const escape = () => fireEvent.keyDown(document, { key: "Escape" });
  const openCommit = async (trackedRemoved = TRACKED) => {
    DeleteProject.mockResolvedValue({ deletedMeetings: 2, trackedRemoved });
    const view = setup();
    await askAcme();
    click("Delete project");
    await screen.findByRole("dialog", { name: "Commit and push the deletion?" });
    return view;
  };

  test("the link shows with projects, is absent without projects or archive, and opens the panel", async () => {
    const a = setup({ projects: [] });
    expect(screen.queryByRole("button", { name: "Delete a project…" })).not.toBeInTheDocument();
    a.unmount();
    const b = setup({ saved: { ...SAVED, checkoutPath: "" } });
    expect(screen.queryByRole("button", { name: "Delete a project…" })).not.toBeInTheDocument();
    b.unmount();
    setup();
    await openPanel();
    expect(screen.queryByRole("button", { name: "Delete a project…" })).not.toBeInTheDocument();
    expect(screen.getByText("Delete a project")).toBeInTheDocument();
    expect(screen.getByText("Deleting a project removes its files and its meetings from your archive folder, and its meetings from Notted.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete project acme" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Delete project globex" })).toBeEnabled();
    expect(screen.getAllByRole("button", { name: "Close" })).toHaveLength(1);
  });

  test("the prefetch disables the row of a recording project; a rejected plan leaves its row enabled", async () => {
    GetProjectDeletePlan.mockImplementation(async (key: string) => {
      if (key === "acme") throw new Error("nope");
      return PLAN(key, { recording: true });
    });
    setup();
    click("Delete a project…");
    expect(await screen.findByText("Recording. Stop the recording first.")).toBeInTheDocument();
    expect(screen.getAllByText("Recording. Stop the recording first.")).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Delete project globex" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Delete project acme" })).toBeEnabled();
  });

  test("a plan fetched on click that reports recording opens no dialog and disables the row", async () => {
    setup();
    await openPanel();
    GetProjectDeletePlan.mockImplementation(async (key: string) => PLAN(key, { recording: true }));
    click("Delete project acme");
    expect(await screen.findByText("Recording. Stop the recording first.")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete project acme" })).toBeDisabled();
  });

  test("the confirm lists files, folders ascending and the meeting line", async () => {
    setup();
    const dialog = await askAcme();
    expect(within(dialog).getByText("Notted deletes these files and folders from your archive folder:")).toBeInTheDocument();
    expect(within(dialog).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      "instructions/projects/acme.md",
      "instructions/people/ana/acme.md",
      "meetings/acme/2026/2026-10-01-sync/meeting.md",
      "meetings/acme/",
      "meetings/acme/2026/",
      "meetings/acme/2026/2026-10-01-sync/",
      "2 meetings in Notted (1 not saved to the archive)",
    ]);
    expect(within(dialog).getByText("Meetings not saved to the archive are gone for good. Everything else stays in the archive's git history.")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Keep project" })).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Delete project" })).toHaveClass("nt-btn--danger");
  });

  test("with no local meetings the confirm has no meeting line", async () => {
    GetProjectDeletePlan.mockImplementation(async (key: string) => PLAN(key, { localMeetings: 0, unsavedMeetings: 0 }));
    setup();
    const dialog = await askAcme();
    const items = within(dialog).getAllByRole("listitem").map((li) => li.textContent);
    expect(items).toHaveLength(6);
    expect(items.some((t) => t?.includes("in Notted"))).toBe(false);
  });

  test("Keep project and Escape each close the dialog and delete nothing", async () => {
    setup();
    const dialog = await askAcme();
    fireEvent.click(within(dialog).getByRole("button", { name: "Keep project" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    click("Delete project acme");
    await screen.findByRole("dialog", { name: "Delete project acme?" });
    escape();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(DeleteProject).not.toHaveBeenCalled();
  });

  test("while DeleteProject is pending both buttons are disabled, Escape keeps the dialog and it is called once", async () => {
    const d = deferred<{ deletedMeetings: number; trackedRemoved: string[] }>();
    DeleteProject.mockReturnValue(d.promise);
    setup();
    const dialog = await askAcme();
    click("Delete project");
    await waitFor(() => expect(within(dialog).getByRole("button", { name: "Keep project" })).toBeDisabled());
    expect(within(dialog).getByRole("button", { name: "Delete project" })).toBeDisabled();
    escape();
    expect(screen.getByRole("dialog", { name: "Delete project acme?" })).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete project" }));
    expect(DeleteProject).toHaveBeenCalledTimes(1);
    expect(DeleteProject).toHaveBeenCalledWith("acme", PLAN("acme"));
    await act(async () => d.resolve({ deletedMeetings: 2, trackedRemoved: [] }));
  });

  test("success with nothing tracked shows the notice, no commit dialog, closes the panel and calls onDeleted once", async () => {
    DeleteProject.mockResolvedValue({ deletedMeetings: 2, trackedRemoved: [] });
    const { onDeleted } = setup();
    await askAcme();
    click("Delete project");
    expect(await screen.findByText("Deleted project acme.")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(onDeleted).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("Delete a project")).not.toBeInTheDocument();
    expect(CommitProjectDeletion).not.toHaveBeenCalled();
  });

  test("success with tracked files asks to commit; Commit and push calls the binding once, disables while pending and ends with the notice", async () => {
    const d = deferred<void>();
    CommitProjectDeletion.mockReturnValue(d.promise);
    await openCommit();
    const dialog = screen.getByRole("dialog", { name: "Commit and push the deletion?" });
    expect(within(dialog).getByText('Notted commits only the removed files, with the message "project(acme): delete", and pushes to origin.')).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Commit and push" })).toHaveClass("nt-btn--primary");
    fireEvent.click(within(dialog).getByRole("button", { name: "Commit and push" }));
    await waitFor(() => expect(within(dialog).getByRole("button", { name: "Not now" })).toBeDisabled());
    expect(within(dialog).getByRole("button", { name: "Commit and push" })).toBeDisabled();
    fireEvent.click(within(dialog).getByRole("button", { name: "Commit and push" }));
    expect(CommitProjectDeletion).toHaveBeenCalledTimes(1);
    expect(CommitProjectDeletion).toHaveBeenCalledWith("acme", TRACKED);
    await act(async () => d.resolve());
    expect(await screen.findByText("Deleted project acme.")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  test("Not now and Escape skip the commit with the not-committed notice", async () => {
    const a = await openCommit();
    click("Not now");
    expect(await screen.findByText("Deleted project acme. The deletion is not committed.")).toBeInTheDocument();
    expect(CommitProjectDeletion).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    a.unmount();
    await openCommit();
    escape();
    expect(await screen.findByText("Deleted project acme. The deletion is not committed.")).toBeInTheDocument();
    expect(CommitProjectDeletion).not.toHaveBeenCalled();
  });

  test("a commit failure keeps the dialog with the reason; Not now then gives the plain notice; a retry that works closes it", async () => {
    const reason = "The commit 1a2b3c4 was saved locally but not pushed:\nno network";
    CommitProjectDeletion.mockRejectedValueOnce(new Error(reason));
    const a = await openCommit();
    click("Commit and push");
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(reason);
    expect(screen.getByRole("dialog", { name: "Commit and push the deletion?" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Not now" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Commit and push" })).toBeEnabled();
    click("Not now");
    expect(await screen.findByText("Deleted project acme.")).toBeInTheDocument();
    expect(screen.queryByText(/not committed/)).not.toBeInTheDocument();
    a.unmount();

    CommitProjectDeletion.mockRejectedValueOnce(new Error(reason));
    await openCommit();
    click("Commit and push");
    await screen.findByRole("alert");
    click("Commit and push");
    expect(await screen.findByText("Deleted project acme.")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(CommitProjectDeletion).toHaveBeenCalledTimes(3);
  });

  test("focus: the commit dialog opens with Not now focused; Not now and Escape return focus to the Delete a project link", async () => {
    const a = await openCommit();
    expect(screen.getByRole("button", { name: "Not now" })).toHaveFocus();
    click("Not now");
    await screen.findByText("Deleted project acme. The deletion is not committed.");
    expect(screen.getByRole("button", { name: "Delete a project…" })).toHaveFocus();
    a.unmount();
    await openCommit();
    expect(screen.getByRole("button", { name: "Not now" })).toHaveFocus();
    escape();
    await screen.findByText("Deleted project acme. The deletion is not committed.");
    expect(screen.getByRole("button", { name: "Delete a project…" })).toHaveFocus();
  });

  test("focus: a successful commit returns focus to the link; a failed one keeps it in the dialog", async () => {
    CommitProjectDeletion.mockRejectedValueOnce(new Error("no network"));
    await openCommit();
    click("Commit and push");
    await screen.findByRole("alert");
    expect(screen.getByRole("dialog")).toContainElement(document.activeElement as HTMLElement);
    click("Commit and push");
    await screen.findByText("Deleted project acme.");
    expect(screen.getByRole("button", { name: "Delete a project…" })).toHaveFocus();
  });

  test("a refused delete closes the dialog, keeps the panel open with the reason, reloads, and Delete… fetches a fresh plan", async () => {
    const reason = 'Project "acme" changed since you reviewed it. Review the list again before deleting.';
    DeleteProject.mockRejectedValueOnce(new Error(reason));
    const { onDeleted } = setup();
    await askAcme();
    GetProjectDeletePlan.mockClear();
    click("Delete project");
    expect(await screen.findByRole("alert")).toHaveTextContent(reason);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(onDeleted).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Delete project acme" })).toBeEnabled();
    click("Delete project acme");
    await screen.findByRole("dialog", { name: "Delete project acme?" });
    expect(GetProjectDeletePlan).toHaveBeenCalledWith("acme");
  });
});

describe("create my file", () => {
  test("shows the button only for a project whose key my files lack", () => {
    setup({ myProjects: ["acme"] });
    expect(screen.getByRole("button", { name: "Create your file for globex" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Create your file for acme" })).not.toBeInTheDocument();
  });

  test("shows no button with myProjects null or with no saved profile", () => {
    const a = setup({ myProjects: null });
    expect(screen.queryByRole("button", { name: /Create your file for/ })).not.toBeInTheDocument();
    a.unmount();
    setup({ myProjects: [], saved: { ...SAVED, profile: "" } });
    expect(screen.queryByRole("button", { name: /Create your file for/ })).not.toBeInTheDocument();
  });

  test("a click creates the file, shows the notice, saves and opens it", async () => {
    const { onSaved, onOpen } = setup({ myProjects: ["acme"] });
    click("Create your file for globex");
    expect(await screen.findByText("Created instructions/people/ana/globex.md. Fill it in below.")).toBeInTheDocument();
    expect(CreateMyProjectFile).toHaveBeenCalledWith("globex");
    expect(onSaved).toHaveBeenCalledTimes(1);
    expect(onOpen).toHaveBeenCalledWith("instructions/people/ana/globex.md");
  });

  test("a rejection shows verbatim in the card's alert", async () => {
    CreateMyProjectFile.mockRejectedValue(new Error("Your file for globex already exists."));
    setup({ myProjects: ["acme"] });
    click("Create your file for globex");
    expect(await screen.findByRole("alert")).toHaveTextContent("Your file for globex already exists.");
  });

  test("myProjectsError shows verbatim and no button exists", () => {
    setup({ myProjects: null, myProjectsError: "Could not list your project files." });
    expect(screen.getByText("Could not list your project files.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Create your file for/ })).not.toBeInTheDocument();
  });
});
