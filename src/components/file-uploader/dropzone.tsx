"use client";

import { useState, type DragEvent } from "react";
import { Icon } from "../icons";

export function Dropzone({ maxSizeMB, onBrowse, onFile }: { maxSizeMB: number; onBrowse: () => void; onFile: (file: File) => void }) {
  const [dragging, setDragging] = useState(false);

  const stop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
  };

  return (
    <div
      className={`uploader-dropzone ${dragging ? "active-drag" : ""}`}
      onDragOver={(e) => { stop(e); setDragging(true); }}
      onDragLeave={(e) => { stop(e); setDragging(false); }}
      onDrop={(e) => {
        stop(e);
        setDragging(false);
        const file = e.dataTransfer.files?.[0];
        if (file) onFile(file);
      }}
      onClick={onBrowse}
      tabIndex={0}
      role="button"
      aria-label="Click or drop file to upload"
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onBrowse();
        }
      }}
    >
      <div className="dropzone-animated-border" />
      <div className="dropzone-content">
        <div className={`floating-icon-wrapper ${dragging ? "pulse" : ""}`}>
          <Icon name="upload" size={26} />
          <div className="icon-glow-ring" />
        </div>
        <div className="dropzone-text">
          <span className="dropzone-primary-action"><strong>Click to browse</strong> or drag &amp; drop file here</span>
          <span className="dropzone-hint">Supported: PNG, JPG, WebP, PDF, PEM · Max {maxSizeMB}MB</span>
        </div>
        <button
          type="button"
          className="button secondary uploader-browse-btn"
          onClick={(e) => { e.stopPropagation(); onBrowse(); }}
        >
          <Icon name="upload" size={15} />Select File
        </button>
      </div>
    </div>
  );
}
