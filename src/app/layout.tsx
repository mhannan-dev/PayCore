import type { Metadata } from "next";
import { AppShell } from "@/components/app-shell";
import { GlobalPreloader } from "@/components/global-preloader";
import { Toaster } from "@/components/toaster";
import "./globals.css";
import "./button-icons.css";
import "./profile.css";
import "./preloader.css";

export const metadata: Metadata = {
  title: { default: "Overview · PayCore", template: "%s · PayCore" },
  description: "PayCore financial operations workspace. Interactive preview with sample data.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en">
      <body>
        <GlobalPreloader />
        {/* Without JavaScript the preloader would never be dismissed. */}
        <noscript><style>{".preloader{display:none}"}</style></noscript>
        <AppShell>{children}</AppShell>
        <Toaster />
      </body>
    </html>
  );
}
