import { useEffect, useState } from "react";
import { DownloadModels, EventsOn, GetModelStatus, type wails } from "../api/bindings";
import { languageName } from "../format";
import "./ModelBanner.css";

type Progress = { phase: string; name: string; bytesDone: number; bytesTotal: number; error?: string };

export function ModelBanner({ language }: { language: string }) {
  const [status, setStatus] = useState<wails.ModelStatusDTO | null>(null);
  const [progress, setProgress] = useState<Progress | null>(null);

  useEffect(() => {
    let live = true;
    setStatus(null);
    setProgress(null);
    GetModelStatus(language)
      .then((s) => live && setStatus(s))
      .catch(() => {});
    const off = EventsOn("models:progress", (p: Progress & { language: string }) => {
      if (p.language === language) setProgress(p);
    });
    return () => {
      live = false;
      off();
    };
  }, [language]);

  if (!status || status.ready || progress?.phase === "done") return null;

  const start = () => {
    setProgress(null);
    DownloadModels(language).catch((e) => setProgress({ phase: "error", name: "", bytesDone: 0, bytesTotal: 0, error: String(e?.message ?? e) }));
  };

  let body: React.ReactNode;
  const phase = progress?.phase;
  if (phase === "download") {
    const pct = progress!.bytesTotal > 0 ? Math.round((progress!.bytesDone / progress!.bytesTotal) * 100) : 0;
    body = (
      <>
        <p>{`Downloading ${progress!.name}… ${pct}%`}</p>
        <div className="nt-bar" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}>
          <div className="nt-bar__fill" style={{ width: `${pct}%` }} />
        </div>
      </>
    );
  } else if (phase === "verify") {
    body = <p>{`Checking ${progress!.name}…`}</p>;
  } else if (phase === "extract") {
    body = <p>{`Unpacking ${progress!.name}…`}</p>;
  } else if (phase === "error") {
    body = (
      <>
        <p>{`The download failed: ${progress!.error ?? ""}`}</p>
        <button className="nt-btn" onClick={start}>
          Try again
        </button>
      </>
    );
  } else {
    body = (
      <>
        <p>{`The speech models for ${languageName(language)} are not on this Mac yet (about ${Math.round(status.downloadBytes / 1e6)} MB). Recording works now; transcription starts once they are downloaded.`}</p>
        <button className="nt-btn nt-btn--primary" onClick={start}>
          Download models
        </button>
      </>
    );
  }

  return (
    <section className="nt-card nt-banner">
      <div className="nt-card__header">Speech models</div>
      {body}
    </section>
  );
}
