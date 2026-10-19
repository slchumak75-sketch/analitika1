# Журнал изменений

## 2026-10-19 — распаковка архива, ревью, автосборка CI
- Получен архив `workspace-01a1539e-e623-7c38-9888-8917305ae93c.zip` (коммит `9f8ccda`),
  проект `retail-breakeven` распакован в корень репозитория.
- Из git удалены: исходный zip (18 МБ), готовый `RetailBreakEven.exe`, кэш модулей Go;
  добавлен `.gitignore`.
- Ревью кода (`docs/CODE_REVIEW.md`): исправлен баг `PotentialSaving` для статей без
  норматива в `calc.go`; добавлен тест `TestPotentialSavingIgnoresNoBenchmark`.
- Добавлен GitHub Actions workflow `.github/workflows/build.yml`: автосборка при каждом
  `push` и `pull_request` (gofmt, vet, тесты, Linux + Windows .exe, артефакты на 30 дней).
- Результат CI: оба автопрогона (push и PR) SUCCESS, артефакт с .exe загружается.
- Документация: переписан `README.md`, добавлены `docs/CODE_REVIEW.md`, `docs/CI.md`,
  `docs/AGENT_HANDOFF.md`, этот журнал.

## До 2026-10-19
- Проект существовал только внутри zip-архива workspace (Go 1.27, версия программы 1.0.0).
- В репозитории был только Initial commit с README-заглушкой.
