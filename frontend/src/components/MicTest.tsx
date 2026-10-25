import { useCallback, useEffect, useRef, useState } from "react";
import { EventsOn, ListMeetings, StartMicTest, StopMicTest } from "../api/bindings";
import { errorMessage } from "../format";
import "./MicTest.css";

type Result = { verdict: string; message: string; stopped: boolean };

const METER_MIN_DB = -60;

// The meter spans -60 dB to 0 dB; anything quieter sits at the left edge.
const meterDb = (db: number) => Math.min(0, Math.max(METER_MIN_DB, Math.round(db)));

// A 10 second level meter on the input macOS uses. Nothing is saved; the app sends the levels and the verdict.
export function MicTest() {
  const [running, setRunning] = useState(false);
  const [level, setLevel] = useState<number | null>(null);
  const [device, setDevice] = useState("");
  const [result, setResult] = useState<Result | null>(null);
  const [error, setError] = useState("");
  const [recording, setRecording] = useState(false);
  const runningRef = useRef(false);
  const live = useRef(true);

  const setRun = useCallback((on: boolean) => {
    runningRef.current = on;
    setRunning(on);
  }, []);

  const checkRecording = useCallback(() => {
    ListMeetings()
      .then((list) => live.current && setRecording((list ?? []).some((m) => m.state === "recording")))
      .catch(() => {});
  }, []);

  useEffect(() => {
    live.current = true;
    checkRecording();
    const offState = EventsOn("recording:state", checkRecording);
    const offLevel = EventsOn("mictest:level", (e: { device: string; rmsDb: number }) => {
      if (!runningRef.current) return;
      setLevel(e.rmsDb);
      if (e.device) setDevice(e.device);
    });
    const offDone = EventsOn("mictest:done", (e: { device: string; verdict: string; message: string; stopped: boolean }) => {
      setRun(false);
      setLevel(null);
      if (e.device) setDevice(e.device);
      setResult({ verdict: e.verdict, message: e.message, stopped: e.stopped });
    });
    const offError = EventsOn("mictest:error", (e: { message: string }) => {
      setRun(false);
      setLevel(null);
      setError(e.message);
    });
    return () => {
      live.current = false;
      offState();
      offLevel();
      offDone();
      offError();
      if (runningRef.current) StopMicTest().catch(() => {});
    };
  }, [checkRecording, setRun]);

  const start = () => {
    setError("");
    setResult(null);
    setDevice("");
    setLevel(null);
    setRun(true);
    StartMicTest().catch((e) => {
      if (!live.current) return;
      setRun(false);
      setError(errorMessage(e));
    });
  };
  const stop = () => {
    StopMicTest().catch((e) => live.current && setError(errorMessage(e)));
  };

  const db = level === null ? METER_MIN_DB : meterDb(level);
  return (
    <div className="nt-mictest">
      {running ? (
        <>
          <div className="nt-mictest__line">
            <span className="nt-mictest__device">{device ? `Listening to ${device}` : "Listening…"}</span>
            <button type="button" className="nt-btn" onClick={stop}>
              Stop
            </button>
          </div>
          <div className="nt-mictest__meter" role="meter" aria-label="Microphone level" aria-valuemin={METER_MIN_DB} aria-valuemax={0} aria-valuenow={db} aria-valuetext={`${db} dB`}>
            <div className="nt-mictest__fill" style={{ width: `${((db - METER_MIN_DB) / -METER_MIN_DB) * 100}%` }} />
          </div>
        </>
      ) : (
        <div className="nt-mictest__line">
          <button type="button" className="nt-btn" onClick={start} disabled={recording}>
            Test microphone
          </button>
          {recording && <span className="nt-mictest__note">Stop the recording to test the microphone.</span>}
        </div>
      )}
      {result && (
        <p role="status" className={`nt-mictest__verdict nt-mictest__verdict--${result.stopped ? "stopped" : result.verdict}`}>
          {result.stopped ? "Test stopped." : result.message}
        </p>
      )}
      {error && (
        <div className="nt-card nt-card--error nt-mictest__error" role="alert">
          {error}
        </div>
      )}
    </div>
  );
}
