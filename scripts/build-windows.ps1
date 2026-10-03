$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force -Path dist | Out-Null
$env:CGO_ENABLED="0"
$env:GOOS="windows"
$env:GOARCH="amd64"
go build -trimpath -ldflags="-H=windowsgui" -o dist/JACoB.exe ./cmd/jacob
go build -trimpath -tags norecorder -ldflags="-H=windowsgui" -o dist/JACoB-no-recorder.exe ./cmd/jacob
