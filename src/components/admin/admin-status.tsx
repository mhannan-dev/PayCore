import { Icon } from "../icons";
import type { AdminState } from "./use-admin-data";

/** Loading / error placeholder for admin pages; renders nothing once data is ready. */
export function AdminStatus<T>({ state, onRetry }: { state: AdminState<T>; onRetry: () => void }) {
  if (state.status === "loading") {
    return (
      <div className="panel adm-status" role="status" aria-live="polite">
        <span className="progress-spinner" aria-hidden="true" />
        Loading access data…
      </div>
    );
  }
  if (state.status === "error") {
    return (
      <div className="panel adm-status is-error" role="alert">
        <Icon name={state.forbidden ? "lock" : "shield"} size={22} />
        <p>{state.message}</p>
        {!state.forbidden && <button type="button" className="button secondary" onClick={onRetry}><Icon name="refresh" size={15} />Try again</button>}
      </div>
    );
  }
  return null;
}
