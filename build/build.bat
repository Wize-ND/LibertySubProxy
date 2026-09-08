@echo off
setlocal enabledelayedexpansion

rem ============================================================
rem  Кросс-сборка libertysubproxy для Keenetic Hopper SE (arm64)
rem  CGO_ENABLED=0 — статический бинарник, не нужен glibc/musl.
rem  -ldflags "-s -w" — срезает таблицы символов (~30% размера).
rem  -trimpath — детерминированная сборка, меньше мусора.
rem ============================================================

set GOOS=linux
set GOARCH=arm64
set CGO_ENABLED=0
set GOFLAGS=

set LDFLAGS=-s -w
echo Building linux/arm64...
go build -trimpath -ldflags "%LDFLAGS%" -o bin\libertysubproxy-arm64 .

if errorlevel 1 (
    echo BUILD FAILED
    exit /b 1
)

rem Дополнительно: сборка под armv7 (Keenetic на 32-битных чипах, старые модели)
set GOARCH=arm
set GOARM=7
echo Building linux/arm/v7...
go build -trimpath -ldflags "%LDFLAGS%" -o bin\libertysubproxy-armv7 .

if errorlevel 1 (
    echo BUILD armv7 FAILED
    exit /b 1
)

rem Сборка под Windows для локальной отладки
set GOOS=windows
set GOARCH=amd64
set GOARM=
if not exist bin\debug mkdir bin\debug
echo Building windows/amd64 (debug)...
go build -trimpath -ldflags "%LDFLAGS%" -o bin\debug\libertysubproxy.exe .

if errorlevel 1 (
    echo BUILD windows FAILED
    exit /b 1
)

echo.
echo OK. Binaries in bin\:
dir /b bin
endlocal
