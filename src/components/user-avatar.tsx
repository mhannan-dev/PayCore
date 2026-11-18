"use client";

import { initials, useAuthSession } from "@/lib/auth-session";
import { useProfilePhotoUrl } from "@/lib/profile-photo";

/**
 * The signed-in user's photo, or their initials while it loads or when none is set.
 * Renders only the inner content; place it inside an avatar container
 * (.operator-avatar, .profile-avatar) that sets size and shape.
 */
export function UserAvatar() {
  const { session } = useAuthSession();
  const url = useProfilePhotoUrl(session.photo, session.photoAttachmentId, session.token);
  if (!url) return <>{initials(session.name)}</>;
  // eslint-disable-next-line @next/next/no-img-element -- object URL / external URL; next/image cannot optimise either
  return <img src={url} alt="" className="avatar-photo" />;
}
