# Журнал изменений

## 2026-10-19 — релизы для скачивания .exe
- Добавлен workflow `.github/workflows/release.yml`: при публикации Release собирает Windows .exe и прикрепляет `RetailBreakEven-Windows.zip`, `.exe` и `SHA256SUMS.txt`.
- `docs/DOWNLOAD.md`: почему Chrome блокирует неподписанный .exe и как скачать/собрать самому.

## 2026-10-19 — импорт и анализ Excel-таблиц (.xls/.xlsx/.csv)
- Найден и проанализирован «ОТЧЕТ ПО ОРГАНИЗАЦИИ мир.xls» (МИРСТРОЙМАРКЕТ, кварталы, маржа 41%, ФОТ — 66% затрат); образцы перенесены в `samples/`. Цифры и выводы: `docs/SPREADSHEET_IMPORT.md`.
- Программа теперь открывает такие таблицы: парсеры XLS(BIFF8/OLE2)/XLSX/CSV на stdlib (`spreadsheet.go`), анализ отчёта (`report.go`), эндпоинт `/api/import`, карточка импорта в UI, кнопка «В расчёт безубыточности».
- Тесты `spreadsheet_test.go` + сквозной тест на реальном `samples/…мир.xls`; по нему исправлены 3 бага (BOF-смещение, «мар» в названии организации, порядок классификации строк). Расчётное ядро не тронуто. CI зелёный.

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
