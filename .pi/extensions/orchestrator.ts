import type { ExtensionAPI, Message } from "@earendil-works/pi-coding-agent";
import { uuidv7 } from "@earendil-works/pi-ai";

export default function(pi: ExtensionAPI) {
  pi.registerCommand("orchestrate", {
    description: "Запустить сервис или дать задачу в новую подессию (пример: /orchestrate vue-front: добавь padding)",
    handler: async (args, ctx) => {
      if (!args || typeof args !== "string" || !args.trim()) {
        ctx.ui.notify(
          "Использование:\n  /orchestrate vue-front          — запустить dev-server\n  /orchestrate backend         — запустить Go\n  /orchestrate vue-front: задача — новая сессия с задачей",
          "info"
        );
        return;
      }
      
      const raw = args.trim();
      
      // Парсим: "vue-front: задача" или просто "vue-front"
      let target: string;
      let task: string | null = null;
      
      if (raw.startsWith("vue-front:") || raw.startsWith("frontend:")) {
        target = "vue-front";
        task = raw.split(":", 2)[1]?.trim() ?? null;
      } else if (raw.startsWith("backend:") || raw.startsWith("go:")) {
        target = "backend";
        task = raw.split(":", 2)[1]?.trim() ?? null;
      } else if (raw.startsWith("vue-front") || raw.startsWith("frontend")) {
        target = "vue-front";
      } else if (raw.startsWith("backend") || raw.startsWith("go")) {
        target = "backend";
      } else {
        ctx.ui.notify(`Неизвестно: "${raw}".\nДоступно: vue-front, backend`, "warning");
        return;
      }
      
      const currentSession = ctx.sessionManager.getSessionFile();
      
      if (target === "vue-front") {
        await ctx.newSession({
          parentSession: currentSession,
          withSession: async (subCtx) => {
            subCtx.ui.notify(`🚀 Новая подсессия для vue-front`, "info");
            
            if (task) {
              // Запускаем задачу в новой сессии автоматически
              subCtx.ui.notify(`⏳ Выполняю: ${task}`, "info");
              
              try {
                const sessionId = uuidv7();
                const messages: Message[] = [
                  { role: "user", content: [{ type: "text", text: task }] },
                ];
                
                const response = await subCtx.modelRegistry.complete(
                  subCtx.model!,
                  { systemPrompt: "Ты ассистент по фронтенду. Работай в проекте /home/visa/IdeaProjects/SCR/frontend. Читай файлы, редактируй код, используй bash.", messages },
                  { signal: undefined, cacheRetention: "none", sessionId }
                );
                
                const text = response.content
                  .filter((c): c is { type: "text"; text: string } => c.type === "text")
                  .map((c) => c.text)
                  .join("\n");
                
                subCtx.ui.notify(`✅ Готово:\n${text}`, "success");
              } catch (err: any) {
                subCtx.ui.notify(`❌ Ошибка: ${err.message ?? err}`, "error");
              }
            } else {
              // Запускаем dev-server
              const { spawn } = await import("node:child_process");
              const { resolve } = await import("node:path");
              
              const cwd = resolve("/home/visa/IdeaProjects/SCR/frontend");
              const proc = spawn("npm", ["run", "dev"], {
                cwd,
                stdio: "inherit",
                env: { ...process.env },
              });
              
              proc.on("error", (err) => {
                subCtx.ui.notify(`❌ Ошибка: ${err.message}`, "error");
              });
              
              await new Promise(r => setTimeout(r, 2000));
              
              subCtx.ui.notify("✅ Frontend запущен", "success");
            }
          },
        });
      } else if (target === "backend") {
        await ctx.newSession({
          parentSession: currentSession,
          withSession: async (subCtx) => {
            subCtx.ui.notify(`🚀 Новая подсессия для backend`, "info");
            
            if (task) {
              subCtx.ui.notify(`⏳ Выполняю: ${task}`, "info");
              
              try {
                const sessionId = uuidv7();
                const messages: Message[] = [
                  { role: "user", content: [{ type: "text", text: task }] },
                ];
                
                const response = await subCtx.modelRegistry.complete(
                  subCtx.model!,
                  { systemPrompt: "Ты ассистент по бэкенду. Работай в проекте /home/visa/IdeaProjects/SCR.", messages },
                  { signal: undefined, cacheRetention: "none", sessionId }
                );
                
                const text = response.content
                  .filter((c): c is { type: "text"; text: string } => c.type === "text")
                  .map((c) => c.text)
                  .join("\n");
                
                subCtx.ui.notify(`✅ Готово:\n${text}`, "success");
              } catch (err: any) {
                subCtx.ui.notify(`❌ Ошибка: ${err.message ?? err}`, "error");
              }
            } else {
              const { spawn } = await import("node:child_process");
              const { resolve } = await import("node:path");
              
              const cwd = resolve("/home/visa/IdeaProjects/SCR");
              const proc = spawn("go", ["run", "main.go"], {
                cwd,
                stdio: "inherit",
                env: { ...process.env },
              });
              
              proc.on("error", (err) => {
                subCtx.ui.notify(`❌ Ошибка: ${err.message}`, "error");
              });
              
              await new Promise(r => setTimeout(r, 1000));
              
              subCtx.ui.notify("✅ Backend запущен", "success");
            }
          },
        });
      }
    },
  });
}
