import { DocsPage } from "@/components/docs/docs-page";
import { USER_GUIDE_MARKDOWN } from "@/content/user-guide";

export const metadata = {
  title: "User guide · PayCore",
  description: "PayCore কাদের জন্য, কেন, আর কীভাবে ব্যবহার করবেন: who PayCore is for, why it exists, and how to use the console and the Quantum testnet checkout.",
};

/** The guide is bundled at build time from docs/user_guide.md (see scripts/sync-docs.mjs). */
export default function Page() {
  return <DocsPage markdown={USER_GUIDE_MARKDOWN} />;
}
