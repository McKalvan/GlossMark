import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  use: { baseURL: process.env.E2E_BASE_URL ?? "http://localhost:3000", storageState: "tests/.auth/patient.json" },
  retries: process.env.CI ? 2 : 0,
});
