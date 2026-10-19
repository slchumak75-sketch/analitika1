# Автосборка (GitHub Actions)

**Файл:** `.github/workflows/build.yml` · **Workflow:** `build`

## Когда запускается автоматически
| Событие | Запуск |
|---|---|
| `push` в **любую** ветку | ✅ автоматически после внесения изменений |
| `pull_request` (создание/обновление) | ✅ |
| Вручную: Actions → build → Run workflow | ✅ (`workflow_dispatch`) |

Ничего включать отдельно не нужно: GitHub Actions активирует workflow сам,
как только файл `.github/workflows/build.yml` попадает в репозиторий.

## Что делает
1. `actions/checkout`, `actions/setup-go` (версия Go берётся из `go.mod`, сейчас `go 1.27`).
2. Проверка `gofmt` (падает, если код не отформатирован).
3. `go vet ./...`
4. `go test ./... -count=1 -v`
5. Сборка Linux-бинарника (контроль компиляции) и **Windows `RetailBreakEven.exe`**
   (`-H windowsgui`, иконка/версия — из `resource_windows_amd64.syso`).
6. Загрузка артефактов `RetailBreakEven-<sha>` (хранятся 30 дней):
   вкладка **Actions → нужный запуск → Artifacts**.

## Как проверить, что всё зелёное
Статус — бейдж в `README.md`, либо `gh run list` / `gh run watch`.
Локально перед пушем: `gofmt -l . && go vet ./... && go test ./... && ./build.sh`

## Известные ограничения
- `.exe` не подписывается (SmartScreen предупреждает — см. README, это ожидаемо).
- Релизы по тегам не настроены: если понадобится, добавить job с
  `softprops/action-gh-release` на `push: tags: ['v*']` и продолжить с этого места.
