$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force -Path dist | Out-Null
$env:CGO_ENABLED="0"
$env:GOOS="windows"
$env:GOARCH="amd64"
go build -trimpath -ldflags="-H=windowsgui" -o dist/JACoB-legacy-windows-amd64.exe ./cmd/jacob
