import { useEffect, useRef, useState } from "react";
import { EventsOn } from "../api/bindings";
import "./LiveWave.css";

/** One live level of a recording track, as the backend sends it (about 15 a second per track). */
export type LevelEvent = { meetingId: string; track: "mic" | "others"; rmsDb: number; peakDb: number };
type Sample = { t: number; a: number };
type Track = LevelEvent["track"];

export const LEVEL_EVENT = "recording:level";
/** How much time the trace shows, right edge is now. */
export const WINDOW_MS = 6000;
/** Frames closer together than this are skipped: the levels arrive at 15 Hz, 30 fps is plenty. */
const FRAME_MS = 32;
/** With reduced motion the trace stands still and is redrawn twice a second. */
export const REDUCED_MS = 500;
const FLOOR_DB = -60;
const CEIL_DB = -6;

/** Maps a level in dBFS to a trace height from 0 (quiet) to 1 (loud). */
export function levelToAmp(rmsDb: number, peakDb: number): number {
  const norm = (db: number) => (Number.isFinite(db) ? Math.min(1, Math.max(0, (db - FLOOR_DB) / (CEIL_DB - FLOOR_DB))) : 0);
  return Math.min(1, 0.8 * norm(rmsDb) + 0.2 * norm(peakDb));
}

/** The envelope at time t, interpolated between samples; 0 before the first and the last value after it. */
export function envelopeAt(hist: readonly Sample[], t: number): number {
  if (hist.length === 0 || t < hist[0].t) return 0;
  for (let i = hist.length - 1; i >= 0; i--) {
    if (hist[i].t <= t) {
      const next = hist[i + 1];
      if (!next) return hist[i].a;
      const k = (t - hist[i].t) / (next.t - hist[i].t);
      return hist[i].a + (next.a - hist[i].a) * k;
    }
  }
  return 0;
}

type Colors = { mic: string; others: string; grid: string };

/** Draws both tracks as stacked phosphor traces: amber for you, warm grey for the others. */
export function drawScope(ctx: CanvasRenderingContext2D, w: number, h: number, now: number, hist: Record<Track, Sample[]>, colors: Colors, phase: number) {
  ctx.clearRect(0, 0, w, h);
  const lane = h / 2;
  // Graticule: a faint centre line per lane and vertical divisions, like a scope's screen.
  ctx.globalAlpha = 0.6;
  ctx.lineWidth = 1;
  ctx.strokeStyle = colors.grid;
  ctx.beginPath();
  for (let i = 1; i < 10; i++) {
    const x = Math.round((w * i) / 10) + 0.5;
    ctx.moveTo(x, 0);
    ctx.lineTo(x, h);
  }
  ctx.moveTo(0, Math.round(lane) + 0.5);
  ctx.lineTo(w, Math.round(lane) + 0.5);
  ctx.stroke();

  (["mic", "others"] as const).forEach((track, li) => {
    const mid = lane * li + lane / 2;
    const room = lane / 2 - 3;
    const color = track === "mic" ? colors.mic : colors.others;
    const shift = li * 1.7;
    const path = () => {
      ctx.beginPath();
      for (let x = 0; x <= w; x += 2) {
        const env = envelopeAt(hist[track], now - (1 - x / w) * WINDOW_MS);
        const carrier = 0.72 * Math.sin(x * 0.11 + phase + shift) + 0.28 * Math.sin(x * 0.29 - phase * 1.6 + shift);
        const y = mid + (0.04 + 0.96 * env) * room * carrier;
        if (x === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      }
    };
    // Phosphor: a wide dim pass for the glow, then the bright beam, fading out toward the past.
    const grad = ctx.createLinearGradient(0, 0, w, 0);
    grad.addColorStop(0, "transparent");
    grad.addColorStop(0.35, color);
    grad.addColorStop(1, color);
    ctx.strokeStyle = grad;
    ctx.globalAlpha = 0.22;
    ctx.lineWidth = 4;
    path();
    ctx.stroke();
    ctx.globalAlpha = 1;
    ctx.lineWidth = 1.5;
    path();
    ctx.stroke();
  });
  ctx.globalAlpha = 1;
}

function useDocumentVisible(): boolean {
  const [visible, setVisible] = useState(() => document.visibilityState !== "hidden");
  useEffect(() => {
    const on = () => setVisible(document.visibilityState !== "hidden");
    document.addEventListener("visibilitychange", on);
    return () => document.removeEventListener("visibilitychange", on);
  }, []);
  return visible;
}

function useReducedMotion(): boolean {
  const query = () => (typeof window.matchMedia === "function" ? window.matchMedia("(prefers-reduced-motion: reduce)") : null);
  const [reduced, setReduced] = useState(() => query()?.matches ?? false);
  useEffect(() => {
    const mq = query();
    if (!mq) return;
    const on = () => setReduced(mq.matches);
    mq.addEventListener?.("change", on);
    return () => mq.removeEventListener?.("change", on);
  }, []);
  return reduced;
}

function readColors(el: Element): Colors {
  const css = getComputedStyle(el);
  const v = (name: string, fallback: string) => css.getPropertyValue(name).trim() || fallback;
  return { mic: v("--accent", "#f2a93b"), others: v("--fg2", "#b6b3ab"), grid: v("--border", "#2a2b2f") };
}

/**
 * The live wave of a recording: your microphone and the others' audio as two oscilloscope traces.
 * It listens to the level stream and animates only while the meeting records and the window is
 * visible; otherwise it holds no listener, no animation frame and no timer.
 */
export function LiveWave({ meetingId, recording }: { meetingId: string; recording: boolean }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const visible = useDocumentVisible();
  const reduced = useReducedMotion();

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!recording || !visible || !canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const hist: Record<Track, Sample[]> = { mic: [], others: [] };
    let colors = readColors(canvas);
    let colorsAt = performance.now();
    let size = { w: 0, h: 0 };
    const fit = () => {
      const dpr = window.devicePixelRatio || 1;
      const w = canvas.clientWidth || 600;
      const h = canvas.clientHeight || 72;
      if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(h * dpr)) {
        canvas.width = Math.round(w * dpr);
        canvas.height = Math.round(h * dpr);
      }
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      size = { w, h };
    };
    fit();
    const ro = typeof ResizeObserver === "function" ? new ResizeObserver(fit) : null;
    ro?.observe(canvas);

    const draw = (now: number) => {
      if (now - colorsAt > 1000) {
        colors = readColors(canvas); // follows a theme switch within a second
        colorsAt = now;
      }
      drawScope(ctx, size.w, size.h, now, hist, colors, reduced ? 0 : now / 140);
    };

    const off = EventsOn(LEVEL_EVENT, (e: LevelEvent) => {
      if (e.meetingId !== meetingId || (e.track !== "mic" && e.track !== "others")) return;
      const now = performance.now();
      const h = hist[e.track];
      h.push({ t: now, a: levelToAmp(e.rmsDb, e.peakDb) });
      while (h.length > 2 && h[1].t < now - WINDOW_MS - 1000) h.shift();
    });

    let raf = 0;
    let timer: ReturnType<typeof setInterval> | undefined;
    let last = -Infinity;
    if (reduced) {
      draw(performance.now());
      timer = setInterval(() => draw(performance.now()), REDUCED_MS);
    } else {
      const frame = (ts: number) => {
        if (ts - last >= FRAME_MS) {
          last = ts;
          draw(ts);
        }
        raf = requestAnimationFrame(frame);
      };
      raf = requestAnimationFrame(frame);
    }
    return () => {
      off();
      cancelAnimationFrame(raf);
      if (timer !== undefined) clearInterval(timer);
      ro?.disconnect();
    };
  }, [meetingId, recording, visible, reduced]);

  if (!recording) return null;
  return (
    <section className="nt-wave" aria-label="Live audio levels">
      <div className="nt-wave__labels" aria-hidden="true">
        <span className="nt-wave__label nt-wave__label--mic" data-label="You" />
        <span className="nt-wave__label" data-label="Others" />
      </div>
      <canvas ref={canvasRef} className="nt-wave__canvas" role="img" aria-label="Live waveform of your microphone and the others' audio" />
    </section>
  );
}
