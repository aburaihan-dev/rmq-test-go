# rmq-go

A small CLI that declares a RabbitMQ queue (**quorum** by default, or **classic**), publishes JSON messages to it at a fixed rate and consumes them back, logging every `SENT` and `RECV` line.

This page covers building the binary; see [Run](#run) for using it.

## Prerequisites

- [Go](https://go.dev/dl/) 1.27.1 or newer (check with `go version`), the version `go.mod` asks for.
- Internet access on the first build: the two dependencies (`amqp091-go`, `godotenv`) are downloaded and cached.

## Build

Run these from the project folder (the one containing `go.mod`).

### For your current machine

```powershell
go build -o rmq-go.exe .
```

On Linux or macOS use `go build -o rmq-go .`. Without `-o`, `go build` names the binary after the module: `rmq-go.exe` on Windows, `rmq-go` elsewhere.

### Smaller release build

```powershell
go build -trimpath -ldflags="-s -w" -o rmq-go.exe .
```

`-s -w` strips the symbol table and debug info, and `-trimpath` removes local file paths from the binary. On Windows x64 this takes the binary from about 9.6 MB to 6.6 MB.

### For another OS or CPU

The app is pure Go, so cross-compiling needs no extra tools: set `GOOS` and `GOARCH`, then build.

| Target | `GOOS` | `GOARCH` | Output name |
|---|---|---|---|
| Windows x64 | `windows` | `amd64` | `rmq-go-windows-amd64.exe` |
| Windows ARM64 | `windows` | `arm64` | `rmq-go-windows-arm64.exe` |
| Linux x64 | `linux` | `amd64` | `rmq-go-linux-amd64` |
| Linux ARM64 | `linux` | `arm64` | `rmq-go-linux-arm64` |
| macOS Intel | `darwin` | `amd64` | `rmq-go-darwin-amd64` |
| macOS Apple silicon | `darwin` | `arm64` | `rmq-go-darwin-arm64` |

PowerShell:

```powershell
$env:GOOS = "linux"; $env:GOARCH = "amd64"
go build -o rmq-go-linux-amd64 .
Remove-Item Env:GOOS, Env:GOARCH    # back to building for this machine
```

bash / zsh:

```bash
GOOS=linux GOARCH=amd64 go build -o rmq-go-linux-amd64 .
```

- Add `CGO_ENABLED=0` for a fully static Linux binary (for example for Alpine or `scratch` containers).
- Copying a Linux or macOS binary over from Windows can drop its executable bit; fix it with `chmod +x rmq-go-linux-amd64`.

## Run

The binary reads its settings from a `.env` file in the folder you run it from; the file is not embedded in the binary.

1. Copy [.env.example](.env.example) to `.env` next to the binary and edit it for your broker. The vhost must already exist.
2. Run the binary from that folder, in a terminal (Ctrl+C stops it):

```powershell
.\rmq-go.exe -rate 5
```

On Linux or macOS: `./rmq-go -rate 5`.

| Flag | Meaning |
|---|---|
| `-rate N` | Messages sent per second, 1-1000 (default 1). |
| `-read-delay D` | Wait a random time between 0 and `D` (for example `500ms` or `2s`, at most `10s`) before each message is read; the `RECV` line shows the wait as `delay=`. Off by default. |

Messages are read one at a time, so a read delay slows the consumer down: if the average wait (`D/2`) is longer than the gap between messages, a backlog builds up in the queue.

The settings in `.env` (everything but the last is required; see [.env.example](.env.example)):

| Key | Meaning |
|---|---|
| `RABBITMQ_HOST`, `RABBITMQ_PORT` | Broker address, for example `localhost` and `5672`. |
| `RABBITMQ_USER`, `RABBITMQ_PASS` | Login. |
| `RABBITMQ_VHOST` | Virtual host; it must already exist. |
| `RABBITMQ_QUEUE` | Queue name; the queue is declared on start if it does not exist. |
| `RABBITMQ_QUEUE_TYPE` | `quorum` (default when unset or empty) or `classic`, in any letter case. Anything else stops the app at start. |

A queue's type cannot change once it exists: starting with a different `RABBITMQ_QUEUE_TYPE` for an existing queue name fails with the broker's `PRECONDITION_FAILED ... inequivalent arg 'x-queue-type'`. Use a new queue name, or delete the old queue yourself (the app never deletes anything).

To try it without building, use `go run . -rate 5`.

To deploy, copy the binary and a `.env` to the target machine; Go is not needed there. Real environment variables take precedence over values in `.env`.

### Running on Linux

A Linux executable has no extension (`.exe` is a Windows convention). The "bin file" is simply the extensionless file you build, `rmq-go-linux-amd64` below. The name is up to you; `-o rmq-go.bin` works the same.

1. On the Linux machine, check its CPU with `uname -m`: `x86_64` means `GOARCH=amd64`, `aarch64` means `GOARCH=arm64`.
2. Build the Linux binary on your machine (PowerShell shown; on Linux or macOS use the bash form from [For another OS or CPU](#for-another-os-or-cpu)):

   ```powershell
   $env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
   go build -trimpath -ldflags="-s -w" -o rmq-go-linux-amd64 .
   Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
   ```

   If Go is installed on the Linux machine itself, run `go build -o rmq-go .` there instead.
3. Copy the binary and your `.env` to the server (Windows line endings in `.env` are fine):

   ```bash
   scp rmq-go-linux-amd64 .env user@server:/opt/rmq-go/
   ```

4. On the server, make it executable and run it from the folder that holds `.env`:

   ```bash
   cd /opt/rmq-go
   chmod +x rmq-go-linux-amd64
   chmod 600 .env    # it holds the broker password
   ./rmq-go-linux-amd64 -rate 5
   ```

If it fails to start: `Permission denied` means the `chmod +x` is missing, and `Exec format error` means the binary was built for a different CPU or OS than the server's (recheck `uname -m`).
