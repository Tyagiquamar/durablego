import { expect, test } from "@playwright/test"

test("demo failure scenes link to their representative workflow evidence", async ({ page }) => {
  await page.goto("/failure-scenes?mode=demo")

  await expect(page.getByRole("heading", { name: "Failure scenes" })).toBeVisible()
  await expect(page.getByText("Stale completion fencing")).toBeVisible()
  await expect(page.getByRole("heading", { name: "Retry exhaustion" })).toBeVisible()
  await expect(page.getByRole("link", { name: "Inspect execution" }).first()).toHaveAttribute("href", /mode=demo/)
})

test("defaults to Demo mode and preserves it through dashboard navigation", async ({ page }) => {
  await page.goto("/")

  await expect(page.getByText("Demo evidence")).toBeVisible()
  await expect(page.getByRole("link", { name: "Demo" })).toHaveAttribute("aria-current", "true")
  await page.getByRole("link", { name: "Executions", exact: true }).click()
  await expect(page).toHaveURL(/\/workflows\?mode=demo/)
  await expect(page.getByRole("heading", { name: "Workflow executions" })).toBeVisible()
})

test("shows lease, fencing, retry, and event evidence for a demo execution", async ({ page }) => {
  await page.goto("/workflows/wf_order_fenced?mode=demo")

  await expect(page.getByText("worker-payments-b", { exact: true })).toBeVisible()
  await expect(page.getByText("Fencing token", { exact: true }).first()).toBeVisible()
  await expect(page.getByText("completion rejected by fencing token 1 after token 2 claim")).toBeVisible()
})

test("keeps the mode selector visible on a mobile viewport", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto("/?mode=demo")

  await expect(page.getByRole("link", { name: "Live" })).toBeVisible()
  await expect(page.getByRole("link", { name: "Failure scenes" })).toBeVisible()
})

test("shows an honest unavailable state in Live mode without demo evidence links", async ({ page }) => {
  await page.goto("/workflows?mode=live")

  await expect(page.getByRole("heading", { name: "Execution data is not available." })).toBeVisible()
  await expect(page.getByRole("link", { name: "Reload this source" })).toHaveAttribute("href", "/workflows?mode=live")
  await page.goto("/failure-scenes?mode=live")
  await expect(page.getByText("Live evidence appears here only when a matching current execution is available.").first()).toBeVisible()
  await expect(page.getByText("Reference sequence only; not observed Live evidence.").first()).toBeVisible()
  await expect(page.getByRole("link", { name: "Inspect execution" })).toHaveCount(0)
})