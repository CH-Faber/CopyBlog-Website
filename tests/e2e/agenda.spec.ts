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
  await expect(page.getByRole("heading", { name: "Agenda AI" })).toBeVisible()
  await expect(page.getByRole("navigation").last()).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath("agenda-mobile.png"), fullPage: true })
})

test("the unified Agenda AI entry answers questions without creating an item", async ({ page }, testInfo) => {
  const turns: { id: string; role: string; content: string; structuredResult?: unknown }[] = []
  await page.route(/\/assistant\/conversations$/, async (route) => {
    if (route.request().method() === "POST") await route.fulfill({ json: { id: "ai-test-conversation" } })
    else await route.continue()
  })
  await page.route(/\/assistant\/conversations\/ai-test-conversation$/, async (route) => {
    if (route.request().method() === "GET") await route.fulfill({ json: { id: "ai-test-conversation", turns } })
    else await route.continue()
  })
  await page.route(/\/assistant\/conversations\/ai-test-conversation\/turns$/, async (route) => {
    turns.push({ id: "user-1", role: "user", content: "我为什么总是难以开始任务？" })
    turns.push({ id: "assistant-1", role: "assistant", content: "可以先把任务拆成一个五分钟内能完成的动作。", structuredResult: { kind: "answer", reply: "可以先把任务拆成一个五分钟内能完成的动作。", intentSummary: "回答如何开始任务", riskLevel: "low", ambiguityQuestions: [], requiresConfirmation: false, actions: [] } })
    await route.fulfill({ json: { requestId: "test-request", kind: "answer", reply: turns[1].content } })
  })
  await page.goto("/agenda/")
  await page.getByLabel("登录密码").fill(process.env.AGENDA_TEST_PASSWORD ?? "agenda-local-test-password")
  await page.getByRole("button", { name: "登录" }).click()
  const input = page.getByRole("textbox", { name: "发送给 Agenda AI" })
  await input.fill("我为什么总是难以开始任务？")
  await page.getByRole("button", { name: "发送" }).click()
  await expect(page.getByText("可以先把任务拆成一个五分钟内能完成的动作。", { exact: true })).toBeVisible()
  await expect(page.getByLabel("快速记录")).toHaveCount(0)
  await page.screenshot({ path: testInfo.outputPath("agenda-ai-entry.png"), fullPage: true })
})
