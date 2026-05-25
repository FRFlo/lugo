set -e

mkdir -p vscode/bin

go run ./cmd/rage-lua-natives -out ./lsp/stdlib

go build -o ./vscode/bin/lugo-linux-x64

chmod +x ./vscode/bin/lugo-linux-x64
