import { useEffect, useState } from "react";
import { ListClients, SetMeetingClient } from "../api/bindings";
import { errorMessage } from "../format";
import "./ClientCard.css";

type Props = { meetingId: string; clientKey: string; hasSession: boolean; onChanged: () => void };

export function ClientCard({ meetingId, clientKey, hasSession, onChanged }: Props) {
  const [clients, setClients] = useState<string[] | null>(null);
  const [listError, setListError] = useState("");
  const [value, setValue] = useState(clientKey);
  const [error, setError] = useState("");

  useEffect(() => setValue(clientKey), [clientKey]);

  useEffect(() => {
    let live = true;
    setClients(null);
    setListError("");
    ListClients()
      .then((list) => live && setClients(list ?? []))
      .catch((e) => live && setListError(errorMessage(e)));
    return () => {
      live = false;
    };
  }, [meetingId]);

  const change = (key: string) => {
    const previous = value;
    setValue(key);
    setError("");
    SetMeetingClient(meetingId, key)
      .then(onChanged)
      .catch((e) => {
        setValue(previous);
        setError(errorMessage(e));
      });
  };

  const options = clients ?? [];
  const shown = value && !options.includes(value) ? [...options, value] : options;

  return (
    <section className="nt-card">
      <div className="nt-card__header">Client</div>
      {listError ? (
        <p className="nt-client__note">{listError}</p>
      ) : clients !== null && clients.length === 0 ? (
        <p className="nt-client__note">No client files found in instructions/clients/ of the archive folder.</p>
      ) : (
        <label className="nt-client__label">
          <span>Client for this meeting</span>
          <select className="nt-select" value={value} disabled={hasSession} onChange={(e) => change(e.target.value)}>
            <option value="">Choose a client</option>
            {shown.map((k) => (
              <option key={k} value={k}>
                {k}
              </option>
            ))}
          </select>
        </label>
      )}
      {hasSession && <p className="nt-client__note">The client is fixed once the summary exists.</p>}
      {error && (
        <div className="nt-card nt-card--error nt-client__error" role="alert">
          {error}
        </div>
      )}
    </section>
  );
}
