"use client";

import { useState, type RefObject } from "react";
import { AuthApiError } from "@/lib/auth-api";
import { removePhoto, updatePhoto } from "@/lib/auth-session";
import { notify } from "@/lib/notify";

// Same limits the API enforces (storage.MaxFileSize, PUT /auth/photo checks).
export const PHOTO_TYPES = ["image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp", "image/tiff"];
const MAX_BYTES = 10 * 1024 * 1024;
const MAX_SIDE = 10000;
const MAX_PIXELS = 25_000_000;

function imageSize(file: File): Promise<{ width: number; height: number }> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => { resolve({ width: img.naturalWidth, height: img.naturalHeight }); URL.revokeObjectURL(url); };
    img.onerror = () => { reject(new Error("unreadable")); URL.revokeObjectURL(url); };
    img.src = url;
  });
}

/** Returns why the file can't be used as a photo, or "" if it looks fine. */
async function checkPhoto(file: File): Promise<string> {
  if (!PHOTO_TYPES.includes(file.type)) return "Choose a JPEG, PNG, GIF, WebP, BMP or TIFF image.";
  if (file.size === 0 || file.size > MAX_BYTES) return "The photo must be smaller than 10 MB.";
  // Browsers may not decode TIFF; the API validates dimensions before conversion.
  if (file.type === "image/tiff") return "";
  try {
    const { width, height } = await imageSize(file);
    if (width > MAX_SIDE || height > MAX_SIDE || width * height > MAX_PIXELS) {
      return "The photo is too large: at most 25 megapixels and 10,000 px per side.";
    }
  } catch {
    return "That image could not be read. Try another file.";
  }
  return "";
}

const message = (error: unknown, fallback: string) => (error instanceof AuthApiError ? error.message : fallback);

/**
 * Photo upload/remove actions and busy state. The caller owns the hidden file
 * input's ref (React's compiler forbids reading refs returned inside render data).
 */
export function usePhotoUpload(inputRef: RefObject<HTMLInputElement | null>) {
  const [busy, setBusy] = useState<"uploading" | "removing" | null>(null);

  async function upload(file: File) {
    const problem = await checkPhoto(file);
    if (problem) return void notify.error(problem);
    setBusy("uploading");
    try {
      await updatePhoto(file);
      notify.success("Profile photo updated.");
    } catch (error) {
      notify.error(message(error, "Could not update your photo. Try again."));
    } finally {
      setBusy(null);
      if (inputRef.current) inputRef.current.value = "";
    }
  }

  async function remove() {
    setBusy("removing");
    try {
      await removePhoto();
      notify.info("Profile photo removed.");
    } catch (error) {
      notify.error(message(error, "Could not remove your photo. Try again."));
    } finally {
      setBusy(null);
    }
  }

  return {
    busy,
    pick: () => inputRef.current?.click(),
    remove,
    onFileChange: (file: File | undefined) => { if (file) void upload(file); },
  };
}
