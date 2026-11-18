"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";
import { usePathname, useRouter } from "next/navigation";
import { AuthApiError, login } from "@/lib/auth-api";
import { startSession } from "@/lib/auth-session";
import { notify } from "@/lib/notify";
import { CaptchaField } from "./login/captcha-field";
import { DEMO_PASSWORD, PRESET_ROLES, SHOW_DEMO_IDENTITIES, type RolePreset } from "./login/demo-identities";
import { DemoIdentityPicker } from "./login/demo-identity-picker";
import { FormMessages, IconField, PasswordField, RememberDevice, SubmitButton } from "./login/form-fields";
import { LoginDisclaimer, LoginFooter, LoginHeader } from "./login/login-header";
import { validateLogin } from "./login/login-schema";
import { useCaptcha } from "./login/use-captcha";

export function LoginPage() {
  const router = useRouter();
  const pathname = usePathname();

  const [email, setEmail] = useState(SHOW_DEMO_IDENTITIES ? PRESET_ROLES[0]?.email ?? "" : "");
  const [password, setPassword] = useState("");
  const [rememberDevice, setRememberDevice] = useState(true);
  const [isLoading, setIsLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");
  const [identityPicks, setIdentityPicks] = useState(0);
  const captcha = useCaptcha();

  const handleRoleSelect = (role: RolePreset) => {
    setEmail(role.email);
    setPassword(DEMO_PASSWORD);
    setIdentityPicks((n) => n + 1); // re-hides the password
    setErrorMessage("");
    requestAnimationFrame(() => document.getElementById("login-password")?.focus());
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const problem = await validateLogin({
      email,
      password,
      hasCaptcha: captcha.captcha !== null,
      captchaAnswer: captcha.answer,
    });
    setErrorMessage(problem);
    if (problem || !captcha.captcha) return;

    setIsLoading(true);
    try {
      const response = await login({
        email: email.trim(),
        password,
        captcha_id: captcha.captcha.captcha_id,
        captcha_answer: captcha.answer,
      });
      startSession(response, rememberDevice);
      setPassword("");
      notify.success(`Welcome back, ${response.user.first_name}.`);
      if (pathname === "/login") router.replace("/");
    } catch (error) {
      setErrorMessage(error instanceof AuthApiError ? error.message : "Sign-in failed. Try again.");
      void captcha.reload();
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <main className="login-page">
      <div className="login-wrapper">
        <section className="login-card" aria-label="Sign-in form">
          <LoginHeader />

          {SHOW_DEMO_IDENTITIES && (
            <DemoIdentityPicker
              roles={PRESET_ROLES}
              selectedEmail={email}
              hasPassword={Boolean(DEMO_PASSWORD)}
              onSelect={handleRoleSelect}
            />
          )}

          <form className="login-form" onSubmit={handleSubmit} noValidate>
            <FormMessages error={errorMessage || captcha.error} />

            <IconField
              id="login-email"
              label="Operator Corporate Email"
              icon="mail"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="operator@yopmail.com"
              required
              autoComplete="email"
              maxLength={254}
            />

            <PasswordField
              id="login-password"
              label="Security Key / Password"
              hint={<Link href="/forgot-password" className="login-forgot-link">Forgot password?</Link>}
              resetKey={String(identityPicks)}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Enter security access password"
              required
              autoComplete="current-password"
            />

            <CaptchaField
              captcha={captcha.captcha}
              answer={captcha.answer}
              loading={captcha.loading}
              disabled={isLoading}
              onAnswerChange={captcha.setAnswer}
              onRefresh={() => void captcha.reload()}
            />

            <RememberDevice checked={rememberDevice} onChange={setRememberDevice} />

            <SubmitButton loading={isLoading} disabled={isLoading || captcha.loading} />
          </form>

          <LoginDisclaimer />
        </section>

        <LoginFooter />
      </div>
    </main>
  );
}
