import { object, string, type InferType } from "yup";

// Mirrors model.UpdateProfileRequest in the API: first name 1–80 characters,
// last name optional (blank is saved as no last name) and at most 80.
export const profileSchema = object({
  firstName: string().trim().required("Enter your first name.").max(80, "First name must be at most 80 characters."),
  lastName: string().trim().max(80, "Last name must be at most 80 characters.").default(""),
});

export type ProfileValues = InferType<typeof profileSchema>;

/** Returns normalized values, or the first validation message. */
export async function validateProfile(input: { firstName: string; lastName: string }): Promise<{ values?: ProfileValues; error?: string }> {
  try {
    return { values: await profileSchema.validate(input, { abortEarly: true }) };
  } catch (error) {
    return { error: error instanceof Error ? error.message : "Check the form and try again." };
  }
}
