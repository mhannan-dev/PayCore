"use client";

import { useCallback, useEffect, useState } from "react";
import { AuthApiError, fetchCaptcha, type Captcha } from "@/lib/auth-api";

const errorText = (error: unknown) =>
  error instanceof AuthApiError ? error.message : "Could not load the security check.";

/** Loads a math challenge on mount; `reload` fetches a fresh one (each login attempt consumes it). */
export function useCaptcha() {
  const [captcha, setCaptcha] = useState<Captcha | null>(null);
  const [answer, setAnswer] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const reload = useCallback(async () => {
    setLoading(true);
    setCaptcha(null);
    setError("");
    setAnswer("");
    try {
      setCaptcha(await fetchCaptcha());
    } catch (error) {
      setCaptcha(null);
      setError(errorText(error));
    } finally {
      setLoading(false);
    }
  }, []);

  // Initial challenge: state starts as loading, so only set it from the async result.
  useEffect(() => {
    let active = true;
    fetchCaptcha()
      .then((value) => { if (active) setCaptcha(value); })
      .catch((error: unknown) => { if (active) setError(errorText(error)); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, []);

  return { captcha, answer, setAnswer, loading, error, reload };
}
