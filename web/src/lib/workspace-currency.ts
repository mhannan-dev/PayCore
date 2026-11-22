"use client";

import { useSyncExternalStore } from "react";
import { CURRENCIES, type Currency } from "./ledger-api";

const storageKey = "paycore.workspace-currency.v1";
const changeEvent = "paycore-workspace-currency-change";
let memoryCurrency: Currency | null = null;

export function parseCurrency(value: unknown): Currency | null {
  return CURRENCIES.find((currency) => currency === value) ?? null;
}

function snapshot(): Currency {
  if (memoryCurrency) return memoryCurrency;
  try {
    return parseCurrency(window.localStorage.getItem(storageKey)) ?? "JPY";
  } catch {
    return "JPY";
  }
}

function subscribe(listener: () => void) {
  const onStorage = () => { memoryCurrency = null; listener(); };
  window.addEventListener("storage", onStorage);
  window.addEventListener(changeEvent, listener);
  return () => {
    window.removeEventListener("storage", onStorage);
    window.removeEventListener(changeEvent, listener);
  };
}

function setCurrency(currency: Currency) {
  memoryCurrency = currency;
  try {
    window.localStorage.setItem(storageKey, currency);
    memoryCurrency = null;
  } catch {
    // Keep switching available for this tab when browser storage is disabled.
  }
  window.dispatchEvent(new Event(changeEvent));
}

/** Browser preference for ledger queries; never an exchange-rate conversion. */
export function useWorkspaceCurrency() {
  const currency = useSyncExternalStore(subscribe, snapshot, (): Currency => "JPY");
  return { currency, setCurrency };
}
