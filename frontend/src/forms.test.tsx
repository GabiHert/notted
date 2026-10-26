import { act, renderHook, waitFor } from "@testing-library/react";
import { useOpenForms, useOpenFormsState } from "./forms";
import { ListOpenForms, emit, resetBindings } from "./test/bindingsMock";

vi.mock("./api/bindings", () => import("./test/bindingsMock"));

beforeEach(() => resetBindings());

const f = (id: string, meetingId: string) => ({ id, meetingId, kind: "ask", title: "T", pieces: [], queued: 0 }) as any;

test("loaded is false until ListOpenForms resolves, then true", async () => {
  let release: (v: unknown[]) => void = () => {};
  ListOpenForms.mockReturnValue(new Promise((r) => (release = r)) as any);
  const { result } = renderHook(() => useOpenFormsState());
  expect(result.current.loaded).toBe(false);
  await act(async () => release([f("f1", "m1")]));
  expect(result.current.loaded).toBe(true);
  expect(result.current.forms.get("m1")?.id).toBe("f1");
});

test("loaded is true after a rejection too, and never goes back", async () => {
  ListOpenForms.mockRejectedValue(new Error("boom"));
  const { result } = renderHook(() => useOpenFormsState());
  await waitFor(() => expect(result.current.loaded).toBe(true));
  ListOpenForms.mockReturnValue(new Promise(() => {}) as any);
  await act(async () => emit("ask:changed", { meetingId: "m1" }));
  expect(result.current.loaded).toBe(true);
});

test("useOpenForms still returns the map keyed by meeting id", async () => {
  ListOpenForms.mockResolvedValue([f("f1", "m1"), f("f2", "m2")] as any);
  const { result } = renderHook(() => useOpenForms());
  await waitFor(() => expect(result.current.size).toBe(2));
  expect(result.current.get("m2")?.id).toBe("f2");
});
