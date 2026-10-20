// @gloss verifies .BookingPage scope=file conf=0.85
import { expect, test } from "@playwright/test";

test.describe("booking page", () => {
  test("a patient books the first open slot and lands on the confirmation", async ({ page }) => {
    await page.goto("/book");
    await page.getByRole("button", { name: "Show times" }).click();
    await page.getByRole("radio").first().check();
    await page.getByRole("button", { name: "Book" }).click();
    await expect(page).toHaveURL(/\/appointments\/[0-9a-f-]+\?booked=1/);
  });

  test("an empty schedule offers the waitlist", async ({ page }) => {
    await page.route("**/slots*", (route) => route.fulfill({ json: [] }));
    await page.goto("/book");
    await expect(page.getByRole("link", { name: "Join the waitlist" })).toBeVisible();
  });
});
