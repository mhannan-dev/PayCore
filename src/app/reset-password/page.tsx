import type { Metadata } from "next";
import { SetPasswordPage } from "@/components/set-password/set-password-page";

export const metadata: Metadata = {
  title: "Reset password",
  description: "Choose a new PayCore password.",
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};

export default function Page() {
  return <SetPasswordPage flow="reset" />;
}
