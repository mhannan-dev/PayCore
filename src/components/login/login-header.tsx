export function LoginHeader() {
  return (
    <>
      <div className="login-header">
        <div className="login-brand">
          <span className="brand-mark">
            p<span>·</span>
          </span>
          PayCore
          <span className="brand-tag">FINANCIAL OS</span>
        </div>
        <span className="login-env-pill">
          <span className="status-dot" />
          SANDBOX v1.0
        </span>
      </div>

      <div className="login-title-group">
        <p className="eyebrow">HIGH-RELIABILITY LEDGER GATEWAY</p>
        <h1>Console Sign-in</h1>
        <p className="login-subtitle">
          Financial operations, append-only double-entry ledger &amp; zero-sum invariant auditor.
        </p>
      </div>
    </>
  );
}

export function LoginFooter() {
  return (
    <footer className="login-page-footer">
      <span>PayCore Financial Ledger Engine · JST (Tokyo, ap-northeast-1)</span>
      <span>ISO 4217 (JPY / USD) · Invariant Verified</span>
    </footer>
  );
}

export function LoginDisclaimer() {
  return (
    <p className="login-disclaimer">
      Sign-in is verified by the PayCore API. Workspace pages still show
      <strong> sample data</strong>; business APIs are not connected yet.
    </p>
  );
}
