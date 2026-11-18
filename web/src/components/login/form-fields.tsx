"use client";

import { useState, type InputHTMLAttributes, type ReactNode } from "react";
import { Icon } from "../icons";

export function FormMessages({ error, notice = "" }: { error: string; notice?: string }) {
  return (
    <>
      {error && <div role="alert" className="error-message">{error}</div>}
      {notice && <div role="status" className="inline-notice">{notice}</div>}
    </>
  );
}

/** Label row: label on the left, optional hint or action on the right. */
export function FieldLabel({ htmlFor, children, aside }: { htmlFor: string; children: ReactNode; aside?: ReactNode }) {
  if (!aside) return <label htmlFor={htmlFor}>{children}</label>;
  return (
    <div className="field-label-row">
      <label htmlFor={htmlFor}>{children}</label>
      {aside}
    </div>
  );
}

type IconInputProps = Omit<InputHTMLAttributes<HTMLInputElement>, "id"> & {
  id: string;
  label: string;
  icon: string;
  hint?: ReactNode;
  trailing?: ReactNode;
};

export function IconField({ id, label, icon, hint, trailing, ...input }: IconInputProps) {
  return (
    <div className="login-field-group">
      <FieldLabel htmlFor={id} aside={hint && <span className="field-hint">{hint}</span>}>
        {label}
      </FieldLabel>
      <div className="input-affix-wrapper">
        <span className="input-icon">
          <Icon name={icon} size={16} />
        </span>
        <input id={id} {...input} />
        {trailing}
      </div>
    </div>
  );
}

/** Password input with its own show/hide toggle; hides again when `resetKey` changes. */
export function PasswordField(props: Omit<IconInputProps, "icon" | "type" | "trailing"> & { resetKey?: string }) {
  const { resetKey, ...rest } = props;
  return <PasswordFieldInner key={resetKey} {...rest} />;
}

function PasswordFieldInner(props: Omit<IconInputProps, "icon" | "type" | "trailing">) {
  const [visible, setVisible] = useState(false);
  return (
    <IconField
      {...props}
      icon="lock"
      type={visible ? "text" : "password"}
      trailing={
        <button
          type="button"
          className="password-toggle-btn"
          onClick={() => setVisible((v) => !v)}
          aria-label={visible ? "Hide password" : "Show password"}
        >
          <Icon name={visible ? "eyeOff" : "eye"} size={16} />
        </button>
      }
    />
  );
}

export function RememberDevice({ checked, onChange }: { checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <div className="login-remember-row">
      <label className="checkbox-label">
        <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
        <span>Remember this terminal session</span>
      </label>
      <span className="sandbox-tag">{checked ? "Stays signed in up to 24h" : "Ends when the tab closes"}</span>
    </div>
  );
}

export function SubmitButton({ loading, disabled }: { loading: boolean; disabled: boolean }) {
  return (
    <button type="submit" disabled={disabled} className="button primary login-submit-btn">
      {loading ? (
        <>
          <span className="spinner" />
          Authenticating terminal...
        </>
      ) : (
        <>
          Authenticate &amp; Open Workspace
          <Icon name="arrow" size={17} />
        </>
      )}
    </button>
  );
}
