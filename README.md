Install all the dependencies:

```sh
go mod tidy
```

Export the target machine as a system variable:

```sh
set GOOS=windows
set GOARCH=amd64
```

Build it with all dependencies:

```sh
go build -ldflags="-s -w" -o touch_event_osc.exe
```