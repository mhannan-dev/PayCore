"use client";

import { useRef } from "react";
import { Icon } from "../icons";
import { UserAvatar } from "../user-avatar";
import { RoleChips } from "./roles";
import { PHOTO_TYPES, usePhotoUpload } from "./use-photo-upload";

type Props = { name: string; email: string; roles: string[]; hasPhoto: boolean };

/** Header card: cover band, large avatar with a camera button, identity, and photo actions. */
export function ProfileHero({ name, email, roles, hasPhoto }: Props) {
  const inputRef = useRef<HTMLInputElement>(null);
  const photo = usePhotoUpload(inputRef);
  const busy = photo.busy !== null;

  return (
    <section className="panel pf-hero" aria-label="Profile overview">
      <div className="pf-cover" aria-hidden="true" />
      <div className="pf-hero-body">
        <div className="pf-avatar-wrap">
          <span className={`pf-avatar${busy ? " is-busy" : ""}`} aria-busy={busy}>
            <UserAvatar />
            {busy && <span className="pf-avatar-spinner" aria-hidden="true"><span className="progress-spinner" /></span>}
          </span>
          <button type="button" className="pf-camera" onClick={photo.pick} disabled={busy} aria-label={hasPhoto ? "Change profile photo" : "Upload profile photo"} title={hasPhoto ? "Change photo" : "Upload photo"}>
            <Icon name="camera" size={15} />
          </button>
        </div>

        <div className="pf-identity">
          <h2>{name}</h2>
          <p className="pf-email"><Icon name="mail" size={13} />{email}</p>
          <RoleChips roles={roles} />
        </div>

        <div className="pf-hero-actions">
          <div className="pf-action-row">
            <button type="button" className="button secondary pf-btn" onClick={photo.pick} disabled={busy}>
              <Icon name="upload" size={15} />
              {photo.busy === "uploading" ? "Uploading…" : hasPhoto ? "Change photo" : "Upload photo"}
            </button>
            {hasPhoto && (
              <button type="button" className="button pf-btn pf-btn-danger" onClick={photo.remove} disabled={busy}>
                <Icon name="trash" size={15} />
                {photo.busy === "removing" ? "Removing…" : "Remove"}
              </button>
            )}
          </div>
          <small className="pf-hint">Up to 10 MB · images are saved as WebP</small>
        </div>
      </div>

      <input
        ref={inputRef}
        type="file"
        accept={PHOTO_TYPES.join(",")}
        className="sr-only"
        tabIndex={-1}
        aria-hidden="true"
        onChange={(e) => photo.onFileChange(e.target.files?.[0])}
      />
    </section>
  );
}
