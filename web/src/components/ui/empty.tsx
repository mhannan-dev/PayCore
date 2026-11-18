import type { ReactNode } from "react";
import { Icon } from "../icons";

export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty-state">{children}</div>;
}

/** Empty state with an icon, a title and one line of guidance. */
export function EmptyHint({ icon, title, text }: { icon: string; title: string; text: string }) {
  return (
    <Empty>
      <Icon name={icon} size={32} />
      <h3>{title}</h3>
      <p>{text}</p>
    </Empty>
  );
}
