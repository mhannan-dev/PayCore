import { boolean, object, string, type InferType } from "yup";

// Mirrors the API's LoginRequest binding (api/internal/models/auth.go).
export const loginSchema = object({
  email: string()
    .trim()
    .required("Please provide a valid operator email address.")
    .email("Please provide a valid operator email address.")
    .max(254, "Email must be at most 254 characters."),
  password: string()
    .required("Please enter your security access key or password.")
    .max(72, "Password must be at most 72 characters."),
  hasCaptcha: boolean().oneOf([true], "The security check is not loaded. Refresh it and try again."),
  captchaAnswer: string()
    .required("Enter the answer to the sum shown in the security check.")
    .matches(/^\d{1,3}$/, "Enter the answer to the sum shown in the security check."),
});

export type LoginValues = InferType<typeof loginSchema>;

/** Returns the first validation message, or "" when the values are valid. */
export async function validateLogin(values: LoginValues): Promise<string> {
  try {
    await loginSchema.validate(values, { abortEarly: true });
    return "";
  } catch (error) {
    return error instanceof Error ? error.message : "Check the form and try again.";
  }
}
