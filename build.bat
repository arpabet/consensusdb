
for /f %%i in ('git describe --tags --always --dirty') do set VER=%%i

echo %VER%

set DATE=%date:~10,4%-%date:~4,2%-%date:~7,2%
echo %DATE%

go test -cover ./...
go build -ldflags "-X main.Version=%VER% -X main.Built=%DATE%"
