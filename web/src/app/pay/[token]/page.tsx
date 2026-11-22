import { Suspense } from "react";
import { CheckoutPage } from "@/components/quantum/checkout-page";

export const metadata = { title: "Testnet checkout" };

export default function Page({ params }: { params: Promise<{ token: string }> }) {
  return (
    <Suspense fallback={<div className="quantum-loading" role="status">Loading checkout…</div>}>
      {params.then(({ token }) => <CheckoutPage token={token} />)}
    </Suspense>
  );
}
