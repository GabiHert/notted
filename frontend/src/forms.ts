import { useEffect, useState } from "react";
import { EventsOn, ListOpenForms, type wails } from "./api/bindings";

// The open form of each meeting, keyed by meeting id. Reloads on every ask:changed.
export function useOpenForms(): Map<string, wails.OpenFormDTO> {
  const [forms, setForms] = useState<Map<string, wails.OpenFormDTO>>(new Map());
  useEffect(() => {
    let live = true;
    const load = () =>
      ListOpenForms()
        .then((list) => {
          if (!live) return;
          setForms(new Map((list ?? []).map((f) => [f.meetingId, f])));
        })
        .catch(() => {});
    load();
    const off = EventsOn("ask:changed", load);
    return () => {
      live = false;
      off();
    };
  }, []);
  return forms;
}
