"use client";

import { useEffect, useState } from "react";
import { fetchProfilePhoto, isExternalPhoto } from "./auth-api";

// The uploaded photo is served by an authenticated endpoint, so it is fetched with
// the token and shown through an object URL. One download per photo is shared by
// every avatar on the page; switching photos revokes the old object URL.
const cache = new Map<string, Promise<string>>();

function loadPhoto(key: string, token: string): Promise<string> {
  const existing = cache.get(key);
  if (existing) return existing;
  for (const [oldKey, oldUrl] of cache) {
    cache.delete(oldKey);
    void oldUrl.then(URL.revokeObjectURL, () => undefined);
  }
  const pending = fetchProfilePhoto(token).then((blob) => URL.createObjectURL(blob));
  pending.catch(() => cache.delete(key)); // allow a retry after a failed download
  cache.set(key, pending);
  return pending;
}

/** Returns a URL an <img> can display for the user's photo, or null (show initials). */
export function useProfilePhotoUrl(photo: string | null, photoAttachmentId: string | null, token: string): string | null {
  const external = isExternalPhoto(photo);
  const key = photo && !external ? photoAttachmentId ?? photo : null;
  const [loaded, setLoaded] = useState<{ key: string; url: string | null } | null>(null);

  useEffect(() => {
    if (!key || !token) return;
    let active = true;
    loadPhoto(key, token).then(
      (url) => { if (active) setLoaded({ key, url }); },
      () => { if (active) setLoaded({ key, url: null }); },
    );
    return () => { active = false; };
  }, [key, token]);

  if (!photo) return null;
  if (external) return photo;
  return loaded?.key === key ? loaded.url : null;
}
