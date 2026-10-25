// Preview-mode stand-in for wailsjs/runtime/runtime: a tiny in-page event bus.
type Handler = (...data: any[]) => void;
const handlers = new Map<string, Set<Handler>>();

export function EventsOn(name: string, cb: Handler): () => void {
  if (!handlers.has(name)) handlers.set(name, new Set());
  handlers.get(name)!.add(cb);
  return () => {
    handlers.get(name)?.delete(cb);
  };
}
export function EventsOff(name: string): void {
  handlers.delete(name);
}
export function emit(name: string, payload: unknown): void {
  for (const cb of [...(handlers.get(name) ?? [])]) cb(payload);
}
export async function ClipboardSetText(_text: string): Promise<boolean> {
  return true;
}
