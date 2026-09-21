import { expect, test } from "@playwright/test"

test("Agenda keeps a complete item lifecycle and renders responsively", async ({ page }, testInfo) => {
  const runId = Date.now().toString(36)
  const projectName = `VermilionVoid ${runId}`
  const itemTitle = `验证完整 Agenda 工作流 ${runId}`
  const memoryText = `晚上默认不安排工作任务。${runId}`

  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto("/agenda/")
  await page.getByLabel("登录密码").fill(process.env.AGENDA_TEST_PASSWORD ?? "agenda-local-test-password")
  await page.getByRole("button", { name: "登录" }).click()
  await expect(page.getByRole("heading", { name: "Agenda" })).toBeVisible()

  await page.getByRole("button", { name: "项目", exact: true }).click()
  await page.getByRole("button", { name: "新项目" }).click()
  await page.getByPlaceholder("项目名称").fill(projectName)
  await page.getByPlaceholder("项目目标（可选）").fill("完成个人工作台")
  await page.getByRole("button", { name: "创建" }).click()
  await expect(page.getByRole("heading", { name: projectName })).toBeVisible()

  await page.getByRole("button", { name: "新增" }).click()
  const itemDialog = page.getByRole("dialog", { name: "新增事项" })
  await itemDialog.getByRole("textbox", { name: "标题", exact: true }).fill(itemTitle)
  await itemDialog.getByLabel("项目").fill(projectName)
  await itemDialog.getByLabel("确定性").selectOption("tentative")
  await itemDialog.getByLabel("优先级").selectOption("2")
  await itemDialog.getByLabel("预计用时（分钟）").fill("45")
  await itemDialog.getByRole("button", { name: "保存" }).click()

  await page.getByRole("button", { name: "今天", exact: true }).click()
  await expect(page.getByRole("heading", { name: "现在最值得做" })).toBeVisible()
  await expect(page.getByText(itemTitle)).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath("agenda-today-desktop.png"), fullPage: true })
  await page.getByRole("article").filter({ hasText: itemTitle }).getByLabel("完成事项").click()
  await page.getByRole("button", { name: "历史", exact: true }).click()
  await expect(page.getByLabel("快速记录")).toHaveCount(0)
  await expect(page.getByText(itemTitle)).toBeVisible()
  await page.getByRole("article").filter({ hasText: itemTitle }).getByTitle("重新打开").click()
  await page.getByRole("button", { name: "今天", exact: true }).click()
  await expect(page.getByText(itemTitle)).toBeVisible()

  await page.getByRole("button", { name: "记忆", exact: true }).click()
  await expect(page.getByLabel("快速记录")).toHaveCount(0)
  page.once("dialog", (dialog) => dialog.accept(memoryText))
  await page.getByRole("button", { name: "添加规则" }).click()
  await expect(page.getByText(memoryText)).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath("agenda-desktop.png"), fullPage: true })

  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole("button", { name: "今天", exact: true }).click()
  await expect(page.getByText("快速记录")).toBeVisible()
  await expect(page.getByRole("navigation").last()).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath("agenda-mobile.png"), fullPage: true })
})

test("quick capture remains reviewable before it becomes an item", async ({ page }, testInfo) => {
  const captureText = `临时安排 ${Date.now().toString(36)}：明天下午检查项目进度`
  await page.route("**/api/organizer/v1/captures/*/parse", async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 1_000))
    await route.continue()
  })

  await page.goto("/agenda/")
  await page.getByLabel("登录密码").fill(process.env.AGENDA_TEST_PASSWORD ?? "agenda-local-test-password")
  await page.getByRole("button", { name: "登录" }).click()
  await page.getByLabel("快速记录").fill(captureText)
  await page.getByRole("button", { name: "交给 Agenda" }).click()
  await expect(page.getByText("已收到", { exact: true })).toBeVisible()
  await expect(page.getByText("AI 正在整理内容，可以继续等待", { exact: true })).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath("agenda-capture-processing.png"), fullPage: true })

  await expect(page.getByRole("heading", { name: "待整理" })).toBeVisible()
  await expect(page.getByLabel("快速记录")).toHaveCount(0)
  await expect(page.getByText(captureText, { exact: true })).toBeVisible()
  await expect(page.getByRole("textbox", { name: "标题", exact: true })).toHaveValue(captureText)
  await page.getByRole("button", { name: "确认并安排" }).click()

  await page.getByRole("button", { name: "今天", exact: true }).click()
  await expect(page.getByText(captureText, { exact: true })).toBeVisible()
})
