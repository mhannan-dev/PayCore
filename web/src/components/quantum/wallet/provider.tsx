"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { defineChain } from "viem";
import { createConfig, injected, unstable_connector, WagmiProvider } from "wagmi";
import type { QuantumConfig } from "@/lib/quantum-api";

type Props = { config: QuantumConfig; children: ReactNode };

export function QuantumWalletProvider({ config, children }: Props) {
  const [queryClient] = useState(() => new QueryClient());
  const [wagmiConfig] = useState(() => {
    const chain = defineChain({
      id: config.chain_id,
      name: `Testnet ${config.chain_id}`,
      nativeCurrency: { name: "Testnet gas", symbol: "TEST", decimals: 18 },
      rpcUrls: { default: { http: [] } },
    });
    const chains = [chain] as const;
    const connector = injected();
    return createConfig({
      chains,
      connectors: [connector],
      transports: { [chain.id]: unstable_connector({ type: "injected" }) },
      multiInjectedProviderDiscovery: false,
      ssr: true,
    });
  });

  return <WagmiProvider config={wagmiConfig}><QueryClientProvider client={queryClient}>{children}</QueryClientProvider></WagmiProvider>;
}
