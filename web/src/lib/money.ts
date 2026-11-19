// Money formatting and parsing in minor units (coding rule 1: never floats).
import type { Currency } from "./ledger-api";

const SCALE: Record<Currency, number> = { JPY: 0, USD: 2, EUR: 2 };
const SYMBOL: Record<Currency, string> = { JPY: "¥", USD: "$", EUR: "€" };

export const currencyScale = (currency: Currency) => SCALE[currency];

/** "123456" (minor units) → "¥123,456" / "$1,234.56"; negative values keep their sign. */
export function formatMoney(minorUnits: string | bigint, currency: Currency): string {
  let value: bigint;
  try {
    value = typeof minorUnits === "bigint" ? minorUnits : BigInt(minorUnits);
  } catch {
    return "—";
  }
  const negative = value < BigInt(0);
  const abs = negative ? -value : value;
  const scale = SCALE[currency];
  const digits = abs.toString().padStart(scale + 1, "0");
  const whole = digits.slice(0, digits.length - scale) || "0";
  const fraction = scale > 0 ? "." + digits.slice(digits.length - scale) : "";
  const grouped = whole.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  return `${negative ? "-" : ""}${SYMBOL[currency]}${grouped}${fraction}`;
}

const MAX_MINOR = BigInt("9223372036854775807");

/**
 * Parses a user-entered amount ("1,250" or "12.34") into minor units ("1250"/"1234").
 * Returns an error message instead when the input is not a positive amount with at
 * most the currency's decimal places.
 */
export function parseMoneyInput(input: string, currency: Currency): { minor: string } | { error: string } {
  const scale = SCALE[currency];
  const cleaned = input.replace(/[,\s]/g, "");
  const pattern = scale === 0 ? /^\d+$/ : new RegExp(`^\\d+(\\.\\d{1,${scale}})?$`);
  if (!pattern.test(cleaned)) {
    return { error: scale === 0 ? `Enter a whole ${currency} amount, e.g. 50000.` : `Enter an amount with at most ${scale} decimals, e.g. 12.50.` };
  }
  const [whole = "0", fraction = ""] = cleaned.split(".");
  const minor = BigInt(whole + fraction.padEnd(scale, "0"));
  if (minor <= BigInt(0)) return { error: "The amount must be more than zero." };
  if (minor > MAX_MINOR) return { error: "The amount is too large." };
  return { minor: minor.toString() };
}
