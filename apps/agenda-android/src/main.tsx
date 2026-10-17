import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { AgendaApp } from "@/components/agenda/AgendaApp"
import { initializeNativeAgenda } from "@/lib/agenda-native"
import "./mobile.css"

async function start() {
  await initializeNativeAgenda()
  const root = document.getElementById("root")
  if (!root) throw new Error("Missing application root")
  createRoot(root).render(<StrictMode><AgendaApp /></StrictMode>)
}

void start().catch((error) => {
  const root = document.getElementById("root")
  if (root) root.textContent = `应用启动失败：${error instanceof Error ? error.message : String(error)}`
})
