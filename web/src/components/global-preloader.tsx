"use client";

import { useEffect, useState } from "react";
import { useAuthSession } from "@/lib/auth-session";

// Keeps the preloader up for at least this long after navigation start, so a
// fast load shows the animation briefly instead of flashing it.
const MIN_VISIBLE_MS = 600;
const FADE_MS = 450;

/**
 * Full-screen animated preloader. It is part of the server HTML, so it covers
 * the first paint, and fades out once the app has hydrated and the saved
 * session is resolved.
 */
export function GlobalPreloader() {
  const { session } = useAuthSession();
  const ready = session.status !== "loading";
  const [phase, setPhase] = useState<"visible" | "leaving" | "gone">("visible");

  useEffect(() => {
    if (!ready) return;
    const wait = Math.max(0, MIN_VISIBLE_MS - performance.now());
    const leave = window.setTimeout(() => setPhase("leaving"), wait);
    const remove = window.setTimeout(() => setPhase("gone"), wait + FADE_MS);
    return () => { window.clearTimeout(leave); window.clearTimeout(remove); };
  }, [ready]);

  if (phase === "gone") return null;

  return (
    <div className={`preloader${phase === "leaving" ? " is-leaving" : ""}`} role="status" aria-live="polite" aria-busy={phase === "visible"}>
      <div className="preloader-stage">
        <div className="preloader-mark" aria-hidden="true">
          <span className="preloader-ring" />
          <span className="preloader-ring preloader-ring-inner" />
          <span className="preloader-logo">p<span>·</span></span>
        </div>
        <div className="preloader-word" aria-hidden="true">
          <strong>PayCore</strong>
          <span>CONSOLE</span>
        </div>
        <div className="preloader-bar" aria-hidden="true"><span /></div>
        <span className="sr-only">Loading PayCore…</span>
      </div>
    </div>
  );
}
