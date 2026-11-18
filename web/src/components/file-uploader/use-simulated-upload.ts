"use client";

import { useEffect, useRef, useState } from "react";
import { formatFileSize, type UploadedFileMeta } from "./types";

const STEPS = [
  { progress: 20, status: "Encrypting file (AES-256)...", delay: 200 },
  { progress: 55, status: "Streaming payload chunks...", delay: 450 },
  { progress: 85, status: "Verifying SHA-256 checksum...", delay: 750 },
  { progress: 100, status: "Audit record committed!", delay: 1050 },
];
const FINISH_DELAY = 350;

/** Staged, client-only upload animation. Nothing is sent to a server. */
export function useSimulatedUpload(maxSizeMB: number, onDone?: (file: UploadedFileMeta) => void) {
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const timers = useRef<ReturnType<typeof setTimeout>[]>([]);

  // Cancel pending steps if the component unmounts mid-upload.
  useEffect(() => () => timers.current.forEach(clearTimeout), []);

  function start(file: File) {
    setError("");
    if (file.size > maxSizeMB * 1024 * 1024) {
      setError(`File size exceeds maximum allowed limit (${maxSizeMB} MB).`);
      return;
    }
    setUploading(true);
    setProgress(0);
    setStatus("Initializing secure pipeline...");
    const previewUrl = file.type.startsWith("image/") ? URL.createObjectURL(file) : undefined;

    timers.current = STEPS.map(({ progress: value, status: text, delay }) =>
      setTimeout(() => {
        setProgress(value);
        setStatus(text);
      }, delay),
    );
    timers.current.push(
      setTimeout(() => {
        setUploading(false);
        onDone?.({
          id: `file_${Date.now()}`,
          name: file.name,
          size: formatFileSize(file.size),
          type: file.type || "application/octet-stream",
          previewUrl,
          uploadedAt: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
        });
      }, (STEPS.at(-1)?.delay ?? 0) + FINISH_DELAY),
    );
  }

  function reset() {
    setProgress(0);
    setError("");
  }

  return { uploading, progress, status, error, start, reset };
}
