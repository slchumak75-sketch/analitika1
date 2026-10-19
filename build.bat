@echo off
chcp 65001 >nul
title Сборка RetailBreakEven.exe
cd /d %~dp0

where go >nul 2>nul
if errorlevel 1 (
  echo [ОШИБКА] Go не найден. Установите Go с https://go.dev/dl/ и запустите снова.
  pause
  exit /b 1
)

echo Прогон тестов...
go test ./...
if errorlevel 1 (
  echo [ОШИБКА] тесты не прошли, сборка остановлена.
  pause
  exit /b 1
)

echo Сборка RetailBreakEven.exe (без консольного окна)...
set CGO_ENABLED=0
go build -trimpath -ldflags "-s -w -H windowsgui" -o RetailBreakEven.exe .
if errorlevel 1 (
  echo [ОШИБКА] сборка не удалась.
  pause
  exit /b 1
)

echo.
echo Готово: RetailBreakEven.exe
pause
