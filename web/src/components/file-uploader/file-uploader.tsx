"use client";

import { useRef, useState } from "react";
import { Dropzone } from "./dropzone";
import type { UploadedFileMeta } from "./types";
import { UploadedFileCard } from "./uploaded-file-card";
import { UploadProgress } from "./upload-progress";
import { useSimulatedUpload } from "./use-simulated-upload";

interface FileUploaderProps {
  onFileAccepted?: (file: UploadedFileMeta) => void;
  onFileRemoved?: (fileId: string) => void;
  label?: string;
  description?: string;
  acceptedTypes?: string[];
  maxSizeMB?: number;
}

export function ModernFileUploader({
  onFileAccepted,
  onFileRemoved,
  label = "Identity & Credentials",
  description = "Upload profile photo, signing key, or FSA administrator clearance certificate.",
  acceptedTypes = ["image/png", "image/jpeg", "image/webp", "application/pdf", ".pem", ".pub"],
  maxSizeMB = 5,
}: FileUploaderProps) {
  const [currentFile, setCurrentFile] = useState<UploadedFileMeta | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const upload = useSimulatedUpload(maxSizeMB, (meta) => {
    setCurrentFile(meta);
    onFileAccepted?.(meta);
  });

  const browse = () => inputRef.current?.click();

  const remove = () => {
    if (currentFile?.previewUrl) URL.revokeObjectURL(currentFile.previewUrl);
    const removedId = currentFile?.id;
    setCurrentFile(null);
    upload.reset();
    if (inputRef.current) inputRef.current.value = "";
    if (removedId) onFileRemoved?.(removedId);
  };

  return (
    <div className="file-uploader-container">
      <div className="uploader-header">
        <div>
          <h3 className="uploader-title">{label}</h3>
          <p className="uploader-desc">{description}</p>
        </div>
        <span className="badge neutral">AES-256 Encrypted</span>
      </div>

      <input
        ref={inputRef}
        type="file"
        className="sr-only"
        accept={acceptedTypes.join(",")}
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (file) upload.start(file);
        }}
      />

      {!currentFile && !upload.uploading && <Dropzone maxSizeMB={maxSizeMB} onBrowse={browse} onFile={upload.start} />}
      {upload.uploading && <UploadProgress status={upload.status} progress={upload.progress} />}
      {currentFile && <UploadedFileCard file={currentFile} onRemove={remove} />}
      {upload.error && <div role="alert" className="error-message uploader-error">{upload.error}</div>}
    </div>
  );
}
