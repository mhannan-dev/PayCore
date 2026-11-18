import { object, ref, string, ValidationError } from "yup";

/** Characters as the API counts them (Unicode code points, not UTF-16 units). */
export const passwordLength = (value: string) => [...value].length;
export const PASSWORD_LENGTH = 10;

// Mirrors service.HashPassword in the API: exactly 10 characters.
const setPasswordSchema = object({
  password: string()
    .required("Choose a password.")
    .test("length", `The password must be exactly ${PASSWORD_LENGTH} characters.`, (v = "") => passwordLength(v) === PASSWORD_LENGTH),
  confirm: string().required("Repeat the password.").oneOf([ref("password")], "The passwords do not match."),
});

/** Returns the first problem, or "" when the passwords are acceptable. */
export function validateNewPassword(password: string, confirm: string): string {
  try {
    setPasswordSchema.validateSync({ password, confirm }, { abortEarly: true });
    return "";
  } catch (error) {
    return error instanceof ValidationError ? error.message : "Check the password and try again.";
  }
}
