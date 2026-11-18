import { array, object, string, ValidationError, type InferType } from "yup";

// Mirrors model.CreateUserRequest.Normalize in the API. The user sets their own password via email.
export const newUserSchema = object({
  firstName: string().trim().required("Enter a first name.").max(80, "First name must be at most 80 characters."),
  lastName: string().trim().max(80, "Last name must be at most 80 characters.").default(""),
  email: string()
    .trim()
    .required("Enter an email address.")
    .email("Enter a valid email address, like name@example.com.")
    .max(254, "Email must be at most 254 characters."),
  roles: array(string().required()).min(1, "Choose at least one role.").required(),
});

export type NewUserValues = InferType<typeof newUserSchema>;
export type NewUserErrors = Partial<Record<keyof NewUserValues, string>>;

/** Validates every field and returns the first message per field ({} when valid). */
export function validateNewUser(values: NewUserValues): NewUserErrors {
  try {
    newUserSchema.validateSync(values, { abortEarly: false });
    return {};
  } catch (error) {
    if (!(error instanceof ValidationError)) return {};
    const errors: NewUserErrors = {};
    for (const issue of error.inner) {
      const field = issue.path?.split(/[.[]/)[0] as keyof NewUserValues | undefined;
      if (field && !errors[field]) errors[field] = issue.message;
    }
    return errors;
  }
}
