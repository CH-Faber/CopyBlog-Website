import { defineConfig } from "@playwright/test"
import { tmpdir } from "node:os"
import { join } from "node:path"

export default defineConfig({
  testDir: "./tests/e2e",
  outputDir: join(tmpdir(), "vermilion-agenda-playwright"),
  timeout: 30_000,
  fullyParallel: false,
  use: {
    baseURL: process.env.AGENDA_TEST_URL ?? "http://127.0.0.1:4322",
    trace: "retain-on-failure",
  },
})
