import { Icon } from "../icons";
import type { UploadedFileMeta } from "./types";

function FileThumbnail({ file }: { file: UploadedFileMeta }) {
  if (file.previewUrl) {
    return (
      <div className="file-preview-thumbnail">
        {/* eslint-disable-next-line @next/next/no-img-element -- local object URL preview */}
        <img src={file.previewUrl} alt={file.name} />
      </div>
    );
  }
  return (
    <div className="file-type-icon-box">
      <Icon name={file.name.endsWith(".pdf") ? "book" : "file"} size={22} />
    </div>
  );
}

export function UploadedFileCard({ file, onRemove }: { file: UploadedFileMeta; onRemove: () => void }) {
  return (
    <div className="uploaded-file-card animate-card-appear">
      <div className="file-info-left">
        <FileThumbnail file={file} />
        <div className="file-meta-details">
          <div className="file-meta-row">
            <strong className="file-name" title={file.name}>{file.name}</strong>
            <span className="badge green"><Icon name="check" size={12} /> Verified</span>
          </div>
          <div className="file-specs">
            <span>{file.size}</span>
            <span>•</span>
            <span>Uploaded at {file.uploadedAt}</span>
          </div>
        </div>
      </div>
      <div className="file-actions-right">
        <button type="button" className="file-action-btn remove" onClick={onRemove} title="Remove file" aria-label="Remove uploaded file">
          <Icon name="trash" size={16} />
        </button>
      </div>
    </div>
  );
}
