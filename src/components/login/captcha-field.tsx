import { Icon } from "../icons";

import type { Captcha } from "@/lib/auth-api";
import { FieldLabel } from "./form-fields";

function CaptchaImage({ captcha, loading, onRetry }: { captcha: Captcha | null; loading: boolean; onRetry: () => void }) {
  return (
    <div className="captcha-image" aria-live="polite">
      {loading ? (
        <span className="captcha-placeholder">Loading…</span>
      ) : captcha ? (
        // eslint-disable-next-line @next/next/no-img-element -- server-generated data URL
        <img src={captcha.image} alt="Math question: type the answer" width={200} height={64} />
      ) : (
        <button type="button" className="captcha-refresh" onClick={onRetry}>
          <Icon name="refresh" size={13} />Unavailable · Retry
        </button>
      )}
    </div>
  );
}

export function CaptchaField({
  captcha,
  answer,
  loading,
  disabled,
  onAnswerChange,
  onRefresh,
}: {
  captcha: Captcha | null;
  answer: string;
  loading: boolean;
  disabled: boolean;
  onAnswerChange: (value: string) => void;
  onRefresh: () => void;
}) {
  return (
    <div className="login-field-group">
      <FieldLabel
        htmlFor="login-captcha"
        aside={
          <button type="button" className="captcha-refresh" onClick={onRefresh} disabled={loading || disabled}>
            <Icon name="refresh" size={13} />New question
          </button>
        }
      >
        Security check
      </FieldLabel>
      <div className="captcha-row">
        <CaptchaImage captcha={captcha} loading={loading} onRetry={onRefresh} />
        <input
          id="login-captcha"
          type="text"
          inputMode="numeric"
          pattern="[0-9]{1,3}"
          maxLength={3}
          value={answer}
          onChange={(e) => onAnswerChange(e.target.value.replace(/\D/g, "").slice(0, 3))}
          placeholder="Answer"
          autoComplete="off"
          required
        />
      </div>
    </div>
  );
}
