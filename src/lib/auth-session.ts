"use client";

import { useSyncExternalStore } from "react";
import {
  AuthApiError,
  deleteAttachment,
  fetchMe,
  logout as revokeSession,
  setProfilePhoto,
  updateMe,
  uploadAttachment,
  type AuthUser,
  type LoginResponse,
} from "./auth-api";

// The bearer token lives in browser storage (localStorage when "remember" is on,
// otherwise sessionStorage). The API is the authentication boundary; this store
// only decides which screen to show.
type StoredSession = {
  token: string;
  expiresAt: number;
  user: AuthUser;
};
export type Session = {
  status: "loading" | "signedOut" | "signedIn";
  signedIn: boolean;
  token: string;
  /** The signed-in user's ID ("" when signed out). */
  userId: string;
  name: string;
  firstName: string;
  lastName: string;
  email: string;
  roles: string[];
  /**
   * Permission codes from the API, or null while unknown (a session stored before
   * RBAC, until /auth/me refreshes it). The API enforces permissions; this only shapes the UI.
   */
  permissions: string[] | null;
  /** Photo URL from the API ("/api/v1/auth/photo" needs the token) or null. */
  photo: string | null;
  /** Changes whenever the photo changes; use it as a cache key. */
  photoAttachmentId: string | null;
};

const storageKey = "paycore.auth-session.v1";
const legacyKey = "paycore.demo-session.v1";
const changeEvent = "paycore-auth-session-change";
const PENDING = "\u0000pending";
let memorySnapshot = "";

function readRaw() {
  try {
    return localStorage.getItem(storageKey) ?? sessionStorage.getItem(storageKey) ?? memorySnapshot;
  } catch {
    return memorySnapshot;
  }
}
function subscribe(listener: () => void) {
  window.addEventListener("storage", listener);
  window.addEventListener(changeEvent, listener);
  return () => {
    window.removeEventListener("storage", listener);
    window.removeEventListener(changeEvent, listener);
  };
}
function write(value: StoredSession | null, remember?: boolean) {
  const raw = value ? JSON.stringify(value) : "";
  memorySnapshot = raw;
  try {
    // Keep the session in whichever storage already holds it unless told otherwise.
    const persistent = remember ?? localStorage.getItem(storageKey) !== null;
    localStorage.removeItem(storageKey);
    sessionStorage.removeItem(storageKey);
    localStorage.removeItem(legacyKey);
    if (raw) (persistent ? localStorage : sessionStorage).setItem(storageKey, raw);
  } catch {
    /* Storage disabled: the in-memory snapshot keeps this tab signed in. */
  }
  window.dispatchEvent(new Event(changeEvent));
}
function parse(raw: string): StoredSession | null {
  if (!raw) return null;
  try {
    const value = JSON.parse(raw) as StoredSession;
    if (typeof value?.token !== "string" || typeof value.expiresAt !== "number" || !value.user?.email) return null;
    if (value.expiresAt <= Date.now()) return null;
    return value;
  } catch {
    return null;
  }
}

export function fullName(user: Pick<AuthUser, "first_name" | "last_name">) {
  return [user.first_name, user.last_name].filter(Boolean).join(" ");
}

export function startSession(response: LoginResponse, remember: boolean) {
  write({ token: response.token, expiresAt: Date.now() + response.expires_in * 1000, user: response.user }, remember);
}

/** Revokes the bearer token on the API, then clears local state. Returns false if the API could not confirm revocation. */
export async function signOut(): Promise<boolean> {
  const current = parse(readRaw());
  let revoked = true;
  if (current) {
    try {
      await revokeSession(current.token);
    } catch (error) {
      // 401 means the session was already invalid server-side, which is the goal.
      revoked = error instanceof AuthApiError && error.status === 401;
    }
  }
  write(null);
  return revoked;
}

/** Re-checks the stored token against /auth/me; clears it if the API rejects it. */
export async function validateSession() {
  const current = parse(readRaw());
  if (!current) return;
  try {
    const user = await fetchMe(current.token);
    const latest = parse(readRaw());
    if (latest?.token === current.token) write({ ...latest, user });
  } catch (error) {
    if (error instanceof AuthApiError && error.status === 401) write(null);
  }
}

export function useAuthSession() {
  const raw = useSyncExternalStore(subscribe, readRaw, () => PENDING);
  const stored = raw === PENDING ? null : parse(raw);
  const session: Session = stored
    ? {
        status: "signedIn",
        signedIn: true,
        token: stored.token,
        userId: stored.user.id,
        name: fullName(stored.user),
        firstName: stored.user.first_name,
        lastName: stored.user.last_name ?? "",
        email: stored.user.email,
        roles: stored.user.roles,
        permissions: stored.user.permissions ?? null,
        photo: stored.user.photo ?? null,
        photoAttachmentId: stored.user.photo_attachment_id ?? null,
      }
    : {
        status: raw === PENDING ? "loading" : "signedOut",
        signedIn: false,
        token: "",
        userId: "",
        name: "",
        firstName: "",
        lastName: "",
        email: "",
        roles: [],
        permissions: [],
        photo: null,
        photoAttachmentId: null,
      };
  return { session, signOut };
}

/**
 * Runs an authenticated API action with the stored token. When it returns an
 * updated user, that user is stored (if the session has not changed meanwhile).
 * A 401 signs the browser out, since the token is no longer valid.
 */
async function withSession<T>(action: (token: string, user: AuthUser) => Promise<{ user: AuthUser; result: T }>): Promise<T> {
  const current = parse(readRaw());
  if (!current) throw new AuthApiError(401, "invalid_credentials", "Your session has ended. Sign in again.");
  try {
    const { user, result } = await action(current.token, current.user);
    const latest = parse(readRaw());
    if (latest?.token === current.token) write({ token: latest.token, expiresAt: latest.expiresAt, user });
    return result;
  } catch (error) {
    if (error instanceof AuthApiError && error.status === 401) write(null);
    throw error;
  }
}

/** Best-effort cleanup of a replaced photo file; failure only leaves an unused attachment. */
async function discardAttachment(token: string, id: string | null | undefined) {
  if (!id) return;
  try {
    await deleteAttachment(token, id);
  } catch {
    /* ignore */
  }
}

/** Saves the signed-in user's names on the API (PATCH /auth/me) and stores the returned user. */
export function updateProfile(firstName: string, lastName: string | null): Promise<AuthUser> {
  return withSession(async (token) => {
    const user = await updateMe(token, { first_name: firstName, last_name: lastName });
    return { user, result: user };
  });
}

/** Uploads an image, makes it the profile photo, then deletes the previous photo file. */
export function updatePhoto(file: File): Promise<AuthUser> {
  return withSession(async (token, before) => {
    const attachment = await uploadAttachment(token, file);
    try {
      await setProfilePhoto(token, attachment.id);
    } catch (error) {
      await discardAttachment(token, attachment.id); // rejected (e.g. not JPEG/PNG/GIF): don't keep the upload
      throw error;
    }
    const user = await fetchMe(token);
    if (before.photo_attachment_id !== attachment.id) await discardAttachment(token, before.photo_attachment_id);
    return { user, result: user };
  });
}

/** Clears the profile photo (back to initials) and deletes its file. */
export function removePhoto(): Promise<AuthUser> {
  return withSession(async (token, before) => {
    await setProfilePhoto(token, null);
    const user = await fetchMe(token);
    await discardAttachment(token, before.photo_attachment_id);
    return { user, result: user };
  });
}

export function initials(name: string) {
  return name.trim().split(/\s+/).slice(0, 2).map((part) => Array.from(part)[0] ?? "").join("").toUpperCase() || "PC";
}
