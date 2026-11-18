"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";
import { object, string, ValidationError } from "yup";
import { AuthApiError, requestPasswordReset } from "@/lib/auth-api";
import { Icon } from "../icons";
import { CaptchaField } from "../login/captcha-field";
import { FormMessages, IconField } from "../login/form-fields";
import { LoginFooter } from "../login/login-header";
import { useCaptcha } from "../login/use-captcha";
import "./set-password.css";

// Mirrors model.PasswordResetRequest in the API.
const schema = object({
  email: string().trim().required("Enter your email address.").email("Enter a valid email address, like name@example.com.").max(254, "Email must be at most 254 characters."),
  captchaAnswer: string().required("Enter the answer to the sum shown in the security check.").matches(/^\d{1,3}$/, "Enter the answer to the sum shown in the security check."),
});

function firstProblem(values: { email: string; captchaAnswer: string }) {
  try {
    schema.validateSync(values, { abortEarly: true });
    return "";
  } catch (error) {
    return error instanceof ValidationError ? error.message : "Check the form and try again.";
  }
}

/** "Forgot password": asks for the email and sends a reset link if the account exists. */
export function ForgotPasswordPage() {
  const captcha = useCaptcha();
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");
  const [sending, setSending] = useState(false);
  const [sentTo, setSentTo] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    const problem = firstProblem({ email, captchaAnswer: captcha.answer }) || (captcha.captcha ? "" : "The security check is not loaded. Refresh it and try again.");
    setError(problem);
    if (problem || !captcha.captcha) return;
    setSending(true);
    try {
      await requestPasswordReset({ email: email.trim(), captcha_id: captcha.captcha.captcha_id, captcha_answer: captcha.answer });
      setSentTo(email.trim());
    } catch (e) {
      setError(e instanceof AuthApiError ? e.message : "Could not send the reset link. Try again.");
      void captcha.reload(); // each challenge is single-use
    } finally {
      setSending(false);
    }
  }

  return (
    <main className="login-page">
      <div className="login-wrapper">
        <section className="login-card sp-card" aria-label="Reset your password">
          <div className="login-header">
            <div className="login-brand">
              <span className="brand-mark">p<span>·</span></span>
              PayCore
              <span className="brand-tag">FINANCIAL OS</span>
            </div>
          </div>

          {sentTo ? (
            <div className="sp-result" role="status">
              <span className="sp-result-icon is-ok"><Icon name="mail" size={28} /></span>
              <h1>Check your email</h1>
              <p>
                If an account exists for <strong>{sentTo}</strong>, we&apos;ve sent a link to choose a new password.
                It works once and expires in 30 minutes. Check your spam folder if it doesn&apos;t arrive.
              </p>
              <Link href="/login" className="button secondary">Back to sign in</Link>
            </div>
          ) : (
            <>
              <div className="login-title-group">
                <p className="eyebrow">PASSWORD RESET</p>
                <h1>Forgot your password?</h1>
                <p className="login-subtitle">Enter your account email and we&apos;ll send you a link to choose a new one.</p>
              </div>
              <form className="login-form" onSubmit={submit} noValidate>
                <FormMessages error={error || captcha.error} />
                <IconField
                  id="fp-email"
                  label="Email"
                  icon="mail"
                  type="email"
                  value={email}
                  onChange={(e) => { setEmail(e.target.value); setError(""); }}
                  placeholder="you@company.com"
                  required
                  autoComplete="email"
                  maxLength={254}
                  autoFocus
                />
                <CaptchaField
                  captcha={captcha.captcha}
                  answer={captcha.answer}
                  loading={captcha.loading}
                  disabled={sending}
                  onAnswerChange={captcha.setAnswer}
                  onRefresh={() => void captcha.reload()}
                />
                <button type="submit" className="button primary login-submit-btn" disabled={sending || captcha.loading}>
                  {sending ? <><span className="spinner" /> Sending link…</> : <>Send reset link <Icon name="arrow" size={17} /></>}
                </button>
                <Link href="/login" className="sp-back-link">Back to sign in</Link>
              </form>
            </>
          )}
        </section>
        <LoginFooter />
      </div>
    </main>
  );
}
