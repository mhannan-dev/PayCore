import type { ReactNode } from "react";
import { Icon } from "../icons";

export function Notice({ icon, children, className = "", role }: { icon?: string; children: ReactNode; className?: string; role?: "status" }) {
  return (
    <div className={`inline-notice ${className}`.trim()} role={role}>
      {icon && <Icon name={icon} />}
      {children}
    </div>
  );
}
