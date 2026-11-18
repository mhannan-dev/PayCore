import type { Metadata } from "next";
import { SetPasswordPage } from "@/components/set-password/set-password-page";

export const metadata: Metadata = {
  title: "Set your password",
  description: "Finish setting up your PayCore account.",
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};

export default function Page() {
  return <SetPasswordPage />;
}
