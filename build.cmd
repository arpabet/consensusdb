@echo off
setlocal

for /f %%i in ('git describe --tags --always --dirty') do set VER=%%i
for /f "tokens=1,2,3 delims= " %%i in ('date /t') do set DATE=%%i

echo %VER%
echo %DATE%

go test -cover ./...
go build -ldflags "-X main.Version=%VER% -X main.Built=%DATE%"
