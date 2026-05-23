mkdir -p vscode/bin

go generate ./lsp

go build -o ./vscode/bin/lugo-linux-x64

chmod +x ./vscode/bin/lugo-linux-x64
