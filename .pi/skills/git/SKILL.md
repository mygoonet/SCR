---
name: git
description: "Git commit/push для SCRP. Использовать при запросах 'commit', 'push', 'закоммить', 'запушить'. Правила: только tracked-изменения, untracked никогда, commit message на английском, ветка master. Выполнять в FORK, чтобы не тратить токены основной сессии."
---

# Git SCRP

## КРИТИЧНО: выполнять в fork

Все git-операции (commit/push/status-разбор) выполняются **в отдельном fork**, а не в основной сессии. Основная сессия только:
1. формулирует задачу для fork;
2. получает краткий structured result;
3. отвечает пользователю одной строкой.

```
fork({ task: "git: ..." }) → результат → ответ пользователю
```

Основная сессия НЕ запускает git-команды сама (кроме `git status --short` для быстрой проверки, если пользователь явно просит посмотреть).

## Правила коммита

1. **Только tracked-изменения.** `git add` — только конкретные изменённые файлы (`M` в `git status`).
2. **Untracked никогда не коммитить.** Файлы `??` (`.cbmignore`, `.opencode/`, `jsondump`, `poadump`, `opencode.json` и любые другие) — молча игнорировать. Если untracked-файл явно нужен в репо — сначала спросить пользователя, потом коммитить отдельно.
3. **Commit message на английском**, формат: `<area>: <what was done>` (imperative, lowercase). Примеры:
   - `monitor: bump FetchNotesAPI attempt budget from 60s to ~120s`
   - `web: add driver field to /api/notes`
4. **Ветка — только `master`.** Никаких feature-веток без явной команды.
5. **Перед push проверить сборку** (для Go-изменений): `go vet ./...` и `go build ./...` должны пройти чисто. Если не прошли — НЕ коммитить, вернуть FAILED.
6. **Одна логическая задача = один коммит.** Если в working tree изменения из разных задач — коммитить отдельными commits по файлам.
7. **Push только после успешного commit.** `git push` в master напрямую (репо github.com/mygoonet/SCR).

## Workflow fork'а

```bash
cd /home/visa/SCRP
git status --short          # 1. что изменилось
git diff --stat             # 2. масштаб изменений
git add <файлы M>           # 3. только tracked, по именам
git commit -m "<area>: ..." # 4. english message
go vet ./... && go build ./...   # 5. если менялись .go
git push                    # 6. в master
```

## Structured result (fork → оркестратору/основной сессии)

```
STATUS: DONE | FAILED | BLOCKED
COMMIT: <hash> <message>
PUSHED: yes | no
FILES: <закоммиченные файлы>
SKIPPED_UNTRACKED: <список ?? файлов, которые не трогали>
ISSUES: <если есть>
```

## Запрещено

- `git add .` / `git add -A` — никогда (захватит untracked).
- `git push --force` — никогда без явной команды пользователя.
- `git reset --hard`, `git checkout -- <file>` — только по явному запросу.
- Коммитить чужие незавершённые изменения без понимания, что это.
