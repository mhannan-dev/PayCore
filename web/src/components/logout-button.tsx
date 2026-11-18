"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { signOut } from "@/lib/auth-session";
import { notify } from "@/lib/notify";
import { Icon } from "./icons";
import { ConfirmationModal } from "./confirmation-modal";

export function LogoutButton({ className, iconSize = 16 }: { className: string; iconSize?: number }) {
  const router = useRouter();
  const [pending, setPending] = useState(false);
  const [confirming, setConfirming] = useState(false);

  async function confirmSignOut() {
    if (pending) return;
    setPending(true);
    const revoked = await signOut();
    if (revoked) {
      notify.success("You have been signed out.");
    } else {
      notify.warning("Signed out on this device, but the server could not be reached to end the session. It expires on its own within 24 hours.");
    }
    router.replace("/login");
  }

  return (
    <>
    <button
      type="button"
      aria-label="Logout"
      className={className}
      disabled={pending}
      aria-busy={pending}
      onClick={() => setConfirming(true)}
    >
      <Icon name="logout" size={iconSize} />
      <span>{pending ? "Signing out…" : "Logout"}</span>
    </button>
    <ConfirmationModal
      open={confirming}
      title="Sign out of PayCore?"
      description="You’ll need to sign in again to access your workspace."
      confirmLabel="Sign out"
      pendingLabel="Signing out…"
      pending={pending}
      onCancel={() => setConfirming(false)}
      onConfirm={confirmSignOut}
    />
    </>
  );
}
