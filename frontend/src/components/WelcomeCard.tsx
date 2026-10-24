import type { wails } from "../api/bindings";
import "./WelcomeCard.css";

const Done = () => <span className="nt-welcome__done">Done</span>;

export function WelcomeCard({ saved }: { saved: wails.SettingsDTO }) {
  return (
    <section className="nt-card">
      <div className="nt-card__header">Set up Notted</div>
      <p className="nt-welcome__body">Notted needs your meetings archive and your profile in it before it can write a summary.</p>
      <ol className="nt-welcome__steps">
        <li>
          Choose your archive folder, or clone it from GitHub.
          {saved.checkoutPath && <Done />}
        </li>
        <li>
          Choose your profile, or create a new one.
          {saved.profile && <Done />}
        </li>
        <li>Add your projects and, if you like, their root folders.</li>
      </ol>
    </section>
  );
}
