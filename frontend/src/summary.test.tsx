import { act, renderHook } from "@testing-library/react";
import { useSummaryRun } from "./summary";
import { emit, resetBindings } from "./test/bindingsMock";

vi.mock("./api/bindings", () => import("./test/bindingsMock"));

beforeEach(() => resetBindings());

const ev = (meetingId: string, pass: string, state: string, message?: string) => ({ meetingId, pass, state, message });
const setup = (onDone = vi.fn()) => ({ onDone, ...renderHook(({ id, cb }) => useSummaryRun(id, cb), { initialProps: { id: "m1", cb: onDone } }) });

test("1: starts idle", () => {
  const { result } = setup();
  expect(result.current.pass).toBeNull();
  expect(result.current.error).toBe("");
});

test("2: running sets the pass, per pass", async () => {
  const { result } = setup();
  await act(async () => emit("summary:state", ev("m1", "summary", "running")));
  expect(result.current.pass).toBe("summary");
  await act(async () => emit("summary:state", ev("m1", "suggestions", "running")));
  expect(result.current.pass).toBe("suggestions");
});

test("3: events of another meeting change nothing", async () => {
  const { result } = setup();
  await act(async () => emit("summary:state", ev("m2", "summary", "running")));
  expect(result.current.pass).toBeNull();
});

test("4: done clears the pass and calls onDone once", async () => {
  const { result, onDone } = setup();
  await act(async () => emit("summary:state", ev("m1", "summary", "running")));
  await act(async () => emit("summary:state", ev("m1", "summary", "done")));
  expect(result.current.pass).toBeNull();
  expect(onDone).toHaveBeenCalledTimes(1);
});

test("5: error clears the pass, keeps the message, and a later running clears it", async () => {
  const { result, onDone } = setup();
  await act(async () => emit("summary:state", ev("m1", "summary", "running")));
  await act(async () => emit("summary:state", ev("m1", "summary", "error", "boom")));
  expect(result.current.pass).toBeNull();
  expect(result.current.error).toBe("boom");
  expect(onDone).not.toHaveBeenCalled();
  await act(async () => emit("summary:state", ev("m1", "summary", "running")));
  expect(result.current.error).toBe("");
});

test("6: switching meetings resets the state", async () => {
  const { result, rerender, onDone } = setup();
  await act(async () => emit("summary:state", ev("m1", "summary", "running")));
  expect(result.current.pass).toBe("summary");
  await act(async () => emit("summary:state", ev("m1", "summary", "error", "boom")));
  expect(result.current.error).toBe("boom");
  rerender({ id: "m2", cb: onDone });
  expect(result.current.pass).toBeNull();
  expect(result.current.error).toBe("");
  await act(async () => emit("summary:state", ev("m1", "summary", "running")));
  expect(result.current.pass).toBeNull();
});

test("7: done calls the latest onDone", async () => {
  const { rerender } = setup();
  const oldCb = vi.fn();
  const newCb = vi.fn();
  rerender({ id: "m1", cb: oldCb });
  rerender({ id: "m1", cb: newCb });
  await act(async () => emit("summary:state", ev("m1", "summary", "done")));
  expect(newCb).toHaveBeenCalledTimes(1);
  expect(oldCb).not.toHaveBeenCalled();
});

test("8: setError sets the error", async () => {
  const { result } = setup();
  act(() => result.current.setError("x"));
  expect(result.current.error).toBe("x");
});
