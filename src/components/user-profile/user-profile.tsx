"use client";

import { useAuthSession } from "@/lib/auth-session";
import { Heading } from "../ui";
import { AccountCard } from "./account-card";
import { ProfileDetailsForm } from "./profile-details-form";
import { ProfileHero } from "./profile-hero";

export function UserProfile() {
  const { session } = useAuthSession();

  return (
    <div className="pf-page">
      <Heading
        eyebrow="YOUR WORKSPACE IDENTITY"
        title="User profile"
        description="Manage your profile photo and personal details."
      />
      <ProfileHero name={session.name} email={session.email} roles={session.roles} hasPhoto={session.photo !== null} />
      <div className="pf-grid">
        <ProfileDetailsForm firstName={session.firstName} lastName={session.lastName} email={session.email} />
        <AccountCard roles={session.roles} permissions={session.permissions} />
      </div>
    </div>
  );
}
