import { Icon } from "../../icons";

/** Loading / error / forbidden placeholder for API-backed panels. */
export function ApiStatus({ loading, error, forbidden, onRetry }: { loading: boolean; error: string; forbidden?: boolean; onRetry: () => void }) {
  if (loading) {
    return <div className="api-status" role="status"><span className="progress-spinner" aria-hidden="true" />Loading…</div>;
  }
  if (!error) return null;
  return (
    <div className={`api-status is-error`} role="alert">
      <Icon name={forbidden ? "lock" : "shield"} size={18} />
      <span>{error}</span>
      {!forbidden && <button type="button" className="text-link" onClick={onRetry}><Icon name="refresh" size={14} />Try again</button>}
    </div>
  );
}
