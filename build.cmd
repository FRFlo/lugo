@echo off

if not exist vscode\bin (
	mkdir vscode\bin
)

go generate ./lsp

go build -o .\vscode\bin\lugo-win32-x64.exe
