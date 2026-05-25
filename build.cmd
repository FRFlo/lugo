@echo off

if not exist vscode\bin (
	mkdir vscode\bin
)

go run .\cmd\rage-lua-natives -out .\lsp\stdlib
if errorlevel 1 exit /b %errorlevel%

go build -o .\vscode\bin\lugo-win32-x64.exe
