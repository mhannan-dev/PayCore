const MAX_BASE_UNITS = BigInt("9223372036854775807");
const INVALID_AMOUNT = "Enter a positive amount using digits only.";

function powerOfTen(exponent: number): bigint {
  let result = BigInt(1);
  for (let i = 0; i < exponent; i++) result *= BigInt(10);
  return result;
}

export function parseTokenAmount(input: string, decimals: number): bigint {
  if (!Number.isInteger(decimals) || decimals < 0 || decimals > 18) throw new Error("This token has an unsupported number of decimals.");
  const text = input.trim();
  if (!/^(?:\d+(?:\.\d*)?|\.\d+)$/.test(text)) throw new Error(INVALID_AMOUNT);
  const [whole = "0", fraction = ""] = text.split(".");
  if (fraction.length > decimals) throw new Error(`Use at most ${decimals} decimal places.`);
  const baseUnits = BigInt(whole) * powerOfTen(decimals) + BigInt(fraction.padEnd(decimals, "0") || "0");
  if (baseUnits <= BigInt(0)) throw new Error("Enter an amount greater than zero.");
  if (baseUnits > MAX_BASE_UNITS) throw new Error("This amount exceeds the maximum supported payment size.");
  return baseUnits;
}

export function baseUnitsToBigInt(baseUnits: string): bigint {
  const text = baseUnits.trim();
  if (!/^\d+$/.test(text)) throw new Error("The invoice amount is not a valid integer amount.");
  const value = BigInt(text);
  if (value <= BigInt(0) || value > MAX_BASE_UNITS) throw new Error("The invoice amount is outside the supported range.");
  return value;
}

export function formatTokenAmount(baseUnits: string | bigint, decimals: number): string {
  if (!Number.isInteger(decimals) || decimals < 0 || decimals > 18) return "—";
  const text = typeof baseUnits === "bigint" ? baseUnits.toString() : baseUnits.trim();
  if (!/^\d+$/.test(text)) return "—";
  const value = BigInt(text);
  if (value > MAX_BASE_UNITS) return "—";
  if (decimals === 0) return value.toString();
  const scale = powerOfTen(decimals);
  const whole = value / scale;
  const fraction = (value % scale).toString().padStart(decimals, "0").replace(/0+$/, "");
  return fraction ? `${whole}.${fraction}` : whole.toString();
}

export function tokenAmountHint(decimals: number): string {
  return decimals === 0 ? "Enter a whole number of tokens." : `Up to ${decimals} decimal places`;
}
