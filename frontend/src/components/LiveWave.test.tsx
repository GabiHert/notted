import { act, render } from "@testing-library/react";
import { LiveWave, LEVEL_EVENT, REDUCED_MS, drawScope, envelopeAt, levelToAmp } from "./LiveWave";
import { emit, listenerCount, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

type Ctx = ReturnType<typeof fakeCtx>;
function fakeCtx() {
  const ys: number[] = [];
  return {
    ys,
    clearRect: vi.fn(),
    beginPath: vi.fn(() => ys.splice(0)),
    moveTo: vi.fn((_x: number, y: number) => ys.push(y)),
    lineTo: vi.fn((_x: number, y: number) => ys.push(y)),
    stroke: vi.fn(),
    setTransform: vi.fn(),
    createLinearGradient: vi.fn(() => ({ addColorStop: vi.fn() })),
    globalAlpha: 1,
    lineWidth: 1,
    strokeStyle: "" as unknown,
  };
}

let ctx: Ctx;
let frames: Map<number, FrameRequestCallback>;
let nextId: number;
let rafSpy: ReturnType<typeof vi.fn>;
let cancelSpy: ReturnType<typeof vi.fn>;
let reducedMotion = false;

function setVisibility(state: "visible" | "hidden") {
  Object.defineProperty(document, "visibilityState", { configurable: true, get: () => state });
  act(() => {
    document.dispatchEvent(new Event("visibilitychange"));
  });
}

function runFrame(ts: number) {
  const pending = [...frames.entries()];
  frames.clear();
  act(() => pending.forEach(([, cb]) => cb(ts)));
}

beforeEach(() => {
  resetBindings();
  ctx = fakeCtx();
  frames = new Map();
  nextId = 1;
  reducedMotion = false;
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockImplementation(() => ctx as unknown as CanvasRenderingContext2D);
  rafSpy = vi.fn((cb: FrameRequestCallback) => {
    const id = nextId++;
    frames.set(id, cb);
    return id;
  });
  cancelSpy = vi.fn((id: number) => frames.delete(id));
  vi.stubGlobal("requestAnimationFrame", rafSpy);
  vi.stubGlobal("cancelAnimationFrame", cancelSpy);
  vi.stubGlobal("matchMedia", (q: string) => ({ matches: q.includes("reduced-motion") && reducedMotion, addEventListener: vi.fn(), removeEventListener: vi.fn() }));
  Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "visible" });
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("LiveWave", () => {
  it("draws nothing and listens to nothing when the meeting is not recording", () => {
    const { container } = render(<LiveWave meetingId="m1" recording={false} />);
    expect(container.querySelector("canvas")).toBeNull();
    expect(listenerCount(LEVEL_EVENT)).toBe(0);
    expect(rafSpy).not.toHaveBeenCalled();
  });

  it("subscribes and animates while recording and visible", () => {
    const { container } = render(<LiveWave meetingId="m1" recording />);
    expect(container.querySelector("canvas")).not.toBeNull();
    expect(listenerCount(LEVEL_EVENT)).toBe(1);
    expect(frames.size).toBe(1);
    runFrame(1000);
    expect(ctx.stroke).toHaveBeenCalled();
    expect(frames.size).toBe(1); // asks for the next frame
  });

  it("unsubscribes and cancels the frame when the window is hidden, and resumes when shown", () => {
    render(<LiveWave meetingId="m1" recording />);
    expect(listenerCount(LEVEL_EVENT)).toBe(1);
    setVisibility("hidden");
    expect(listenerCount(LEVEL_EVENT)).toBe(0);
    expect(cancelSpy).toHaveBeenCalled();
    expect(frames.size).toBe(0);
    const requested = rafSpy.mock.calls.length;
    setVisibility("visible");
    expect(listenerCount(LEVEL_EVENT)).toBe(1);
    expect(rafSpy.mock.calls.length).toBe(requested + 1);
  });

  it("unsubscribes and cancels the frame when the recording stops", () => {
    const { rerender, container } = render(<LiveWave meetingId="m1" recording />);
    rerender(<LiveWave meetingId="m1" recording={false} />);
    expect(listenerCount(LEVEL_EVENT)).toBe(0);
    expect(cancelSpy).toHaveBeenCalled();
    expect(frames.size).toBe(0);
    expect(container.querySelector("canvas")).toBeNull();
  });

  it("does not subscribe when it mounts in a hidden window", () => {
    Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "hidden" });
    render(<LiveWave meetingId="m1" recording />);
    expect(listenerCount(LEVEL_EVENT)).toBe(0);
    expect(rafSpy).not.toHaveBeenCalled();
  });

  it("draws a taller trace for loud levels than for silence, and ignores other meetings", () => {
    const now = vi.spyOn(performance, "now");
    render(<LiveWave meetingId="m1" recording />);
    const swing = () => {
      // The newest stretch of the first (mic) trace is at the right edge.
      const ys = ctx.ys.slice(-60);
      return Math.max(...ys) - Math.min(...ys);
    };
    const drawMic = (ts: number) => {
      ctx.ys.splice(0);
      // Capture the mic beam: it is the second path drawn per frame (after its glow pass).
      const paths: number[][] = [];
      ctx.beginPath.mockImplementation(() => {
        paths.push([]);
        return [] as never;
      });
      ctx.lineTo.mockImplementation((_x: number, y: number) => {
        paths[paths.length - 1].push(y);
        return undefined as never;
      });
      runFrame(ts);
      ctx.ys.splice(0, ctx.ys.length, ...paths[2]); // [grid, mic glow, mic beam, others glow, others beam]
    };

    now.mockReturnValue(1000);
    act(() => emit(LEVEL_EVENT, { meetingId: "m1", track: "mic", rmsDb: -120, peakDb: -120 }));
    drawMic(1000);
    const quiet = swing();

    now.mockReturnValue(1100);
    act(() => emit(LEVEL_EVENT, { meetingId: "other", track: "mic", rmsDb: -6, peakDb: 0 }));
    drawMic(1100);
    expect(swing()).toBeLessThan(quiet * 3); // another meeting's level changed nothing

    now.mockReturnValue(1200);
    act(() => emit(LEVEL_EVENT, { meetingId: "m1", track: "mic", rmsDb: -6, peakDb: 0 }));
    drawMic(1300);
    expect(swing()).toBeGreaterThan(quiet * 5);
  });

  it("with reduced motion, redraws a still trace on a slow timer instead of every frame", () => {
    vi.useFakeTimers();
    reducedMotion = true;
    const { rerender } = render(<LiveWave meetingId="m1" recording />);
    expect(rafSpy).not.toHaveBeenCalled();
    expect(listenerCount(LEVEL_EVENT)).toBe(1);
    const drawn = ctx.clearRect.mock.calls.length;
    expect(drawn).toBe(1);
    act(() => vi.advanceTimersByTime(REDUCED_MS * 4));
    expect(ctx.clearRect.mock.calls.length).toBe(drawn + 4);
    rerender(<LiveWave meetingId="m1" recording={false} />);
    act(() => vi.advanceTimersByTime(REDUCED_MS * 4));
    expect(ctx.clearRect.mock.calls.length).toBe(drawn + 4);
  });
});

describe("level helpers", () => {
  it("maps dBFS to a 0..1 height", () => {
    expect(levelToAmp(-120, -120)).toBe(0);
    expect(levelToAmp(0, 0)).toBe(1);
    const mid = levelToAmp(-33, -20);
    expect(mid).toBeGreaterThan(0.3);
    expect(mid).toBeLessThan(0.8);
    expect(levelToAmp(Number.NaN, Number.NaN)).toBe(0);
  });

  it("interpolates the envelope between samples", () => {
    const h = [
      { t: 0, a: 0 },
      { t: 100, a: 1 },
    ];
    expect(envelopeAt([], 50)).toBe(0);
    expect(envelopeAt(h, -1)).toBe(0);
    expect(envelopeAt(h, 50)).toBeCloseTo(0.5);
    expect(envelopeAt(h, 500)).toBe(1);
  });

  it("draws the graticule and two traces, each with a glow and a beam", () => {
    const c = fakeCtx();
    drawScope(c as unknown as CanvasRenderingContext2D, 200, 72, 0, { mic: [], others: [] }, { mic: "#f2a93b", others: "#b6b3ab", grid: "#2a2b2f" }, 0);
    expect(c.clearRect).toHaveBeenCalledWith(0, 0, 200, 72);
    expect(c.stroke).toHaveBeenCalledTimes(5);
    expect(c.createLinearGradient).toHaveBeenCalledTimes(2);
  });
});
