export function UploadProgress({ status, progress }: { status: string; progress: number }) {
  return (
    <div className="uploader-progress-card">
      <div className="progress-card-top">
        <div className="progress-status-group">
          <span className="progress-spinner" />
          <div>
            <strong className="progress-headline">{status}</strong>
            <span className="progress-sub">Secure multi-part transport</span>
          </div>
        </div>
        <span className="progress-percent-badge">{progress}%</span>
      </div>
      <div className="progress-bar-track">
        <div className="progress-bar-fill animated-shimmer" style={{ width: `${progress}%` }} />
      </div>
    </div>
  );
}
