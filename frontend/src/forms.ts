import { useEffect, useState } from "react";
import { EventsOn, ListOpenForms, type wails } from "./api/bindings";

// The open form of each meeting, keyed by meeting id. Reloads on every ask:changed.
export function useOpenForms(): Map<string, wails.OpenFormDTO> {
  return useOpenFormsState().forms;
}

// The same map, and whether the first load has settled (resolved or rejected); it never goes back to false.
export function useOpenFormsState(): { forms: Map<string, wails.OpenFormDTO>; loaded: boolean } {
  const [forms, setForms] = useState<Map<string, wails.OpenFormDTO>>(new Map());
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    let live = true;
    const load = () =>
      ListOpenForms()
        .then((list) => {
          if (!live) return;
          setForms(new Map((list ?? []).map((f) => [f.meetingId, f])));
          setLoaded(true);
        })
        .catch(() => {
          if (live) setLoaded(true);
        });
    load();
    const off = EventsOn("ask:changed", load);
    return () => {
      live = false;
      off();
    };
  }, []);
  return { forms, loaded };
}
