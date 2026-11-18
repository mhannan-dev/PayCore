import type { Metadata } from "next";
import { ForgotPasswordPage } from "@/components/set-password/forgot-password-page";

export const metadata: Metadata = {
  title: "Forgot password",
  description: "Get a link to reset your PayCore password.",
  robots: { index: false, follow: false },
};

export default function Page() {
  return <ForgotPasswordPage />;
}
