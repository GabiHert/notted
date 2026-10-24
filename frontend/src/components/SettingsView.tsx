import { useCallback, useEffect, useRef, useState } from "react";
import { ListClients } from "../api/bindings";
import { errorMessage } from "../format";
import { InstructionFiles } from "./InstructionFiles";
import { PromptPreview } from "./PromptPreview";
import { SetupForm } from "./SetupForm";
import "./SettingsView.css";

export function SettingsView({ onDirtyPath }: { onDirtyPath?: (path: string | null) => void } = {}) {
  const [clients, setClients] = useState<string[]>([]);
  const [clientsError, setClientsError] = useState("");
  const request = useRef(0);

  const loadClients = useCallback(() => {
    const id = ++request.current;
    ListClients()
      .then((list) => {
        if (request.current !== id) return;
        setClients(list ?? []);
        setClientsError("");
      })
      .catch((e) => {
        if (request.current !== id) return;
        setClients([]);
        setClientsError(errorMessage(e));
      });
  }, []);

  useEffect(() => {
    loadClients();
    return () => {
      request.current = -1;
    };
  }, [loadClients]);

  return (
    <div className="nt-settings">
      <h1>Settings</h1>
      <div className="nt-tabs" role="tablist">
        <button className="nt-tabs__tab nt-tabs__tab--selected" role="tab" aria-selected="true" id="nt-tab-instructions" aria-controls="nt-panel-instructions">
          Instructions
        </button>
      </div>
      <div role="tabpanel" id="nt-panel-instructions" aria-labelledby="nt-tab-instructions">
        <SetupForm clients={clients} clientsError={clientsError} onSaved={loadClients} />
        <InstructionFiles onDirtyPath={onDirtyPath} />
        <PromptPreview clients={clients} clientsError={clientsError} />
      </div>
    </div>
  );
}
