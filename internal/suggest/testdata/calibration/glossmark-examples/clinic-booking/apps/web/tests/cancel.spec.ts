// @gloss verifies .AppointmentPage scope=file conf=0.85
import { expect, test } from "@playwright/test";

test("cancelling inside 24 hours warns about a late cancellation", async ({ page }) => {
  await page.goto("/appointments/00000000-0000-4000-8000-000000000123");
  await expect(page.getByText("will count as a late cancellation")).toBeVisible();
  await page.getByRole("button", { name: "Cancel appointment" }).click();
  await expect(page.getByRole("status")).toHaveText("Your appointment was cancelled.");
});
