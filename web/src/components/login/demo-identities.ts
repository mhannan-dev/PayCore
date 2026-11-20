export interface RolePreset {
  id: string;
  name: string;
  email: string;
  roleTitle: string;
  roles: string[];
  badge: "green" | "amber" | "neutral";
}

// Mirrors api/database/seeders/users.go SampleUsers(). The seeder has no built-in
// password. Development autofill is configured only through local environment.
export const PRESET_ROLES: RolePreset[] = [
  { id: "admin", name: "Admin Admin", email: "admin@yopmail.com", roleTitle: "Admin", roles: ["ADMIN", "OPERATOR"], badge: "green" },
  { id: "operator", name: "Operator", email: "operator@yopmail.com", roleTitle: "Operator", roles: ["OPERATOR"], badge: "green" },
  { id: "auditor", name: "Auditor Auditor", email: "auditor@yopmail.com", roleTitle: "Auditor", roles: ["COMPLIANCE_AUDITOR"], badge: "amber" },
  { id: "engineer", name: "Engineer Engineer", email: "engineer@yopmail.com", roleTitle: "Engineer", roles: ["STAFF_ENGINEER"], badge: "neutral" },
];

export const SHOW_DEMO_IDENTITIES = process.env.NODE_ENV === "development";
export const DEMO_PASSWORD = SHOW_DEMO_IDENTITIES ? process.env.NEXT_PUBLIC_DEMO_PASSWORD ?? "" : "";
