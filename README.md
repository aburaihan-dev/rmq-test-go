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

To deploy, copy the binary and a `.env` to the target machine; Go is not needed there. Real environment variables take precedence over values in `.env`.

### Examples

Run these from the folder that holds the binary and `.env`. Ctrl+C stops any of them.

Send one message per second (the default rate):

```powershell
.\rmq-go.exe
```

Send 10 messages per second:

```powershell
.\rmq-go.exe -rate 10
```

Simulate a slow consumer: send 10 per second, but wait a random time of up to 400 ms before reading each message. The consumer then averages about 5 messages per second, so a backlog builds up in the queue:

```powershell
.\rmq-go.exe -rate 10 -read-delay 400ms
```

Run straight from the source, without building (flags go after the `.`):

```powershell
go run . -rate 5 -read-delay 1s
```

On Linux (see [Running on Linux](#running-on-linux)):

```bash
./rmq-go-linux-amd64 -rate 5 -read-delay 500ms
```

Use a classic queue instead of a quorum queue by setting the type in `.env` (the other settings stay as they are):

```
RABBITMQ_QUEUE=demo.classic
RABBITMQ_QUEUE_TYPE=classic
```

Or try the other type for a single run without editing `.env`. Real environment variables win over `.env`, and the queue needs a new name because the type of an existing queue cannot change:

```powershell
$env:RABBITMQ_QUEUE = "demo.classic"; $env:RABBITMQ_QUEUE_TYPE = "classic"
.\rmq-go.exe -rate 5
Remove-Item Env:RABBITMQ_QUEUE, Env:RABBITMQ_QUEUE_TYPE    # PowerShell keeps them for the whole session
```

```bash
RABBITMQ_QUEUE=demo.classic RABBITMQ_QUEUE_TYPE=classic ./rmq-go-linux-amd64 -rate 5
```

#### What the output looks like

`.\rmq-go.exe -rate 5` prints something like this (timestamps and random strings differ on every run):

```
2026/10/06 16:04:11.379588 INFO connected host=localhost:5672 vhost=demo
2026/10/06 16:04:11.395397 INFO queue ready name=demo.quorum type=quorum
2026/10/06 16:04:11.399672 INFO producing; press Ctrl+C to stop rate=5/s
2026/10/06 16:04:11.400690 INFO SENT random=RA2IQSK5QS6Z6SIU74COCFLCXP ts=2026-10-06T16:04:11.400+06:00
2026/10/06 16:04:11.409385 INFO RECV random=RA2IQSK5QS6Z6SIU74COCFLCXP ts=2026-10-06T16:04:11.400+06:00
...
2026/10/06 16:04:17.329440 INFO stopping
```

Each `SENT` line pairs with the `RECV` line that has the same `random=` value, and `ts=` is when the message was created. With `-read-delay`, the startup log says the option is on and each `RECV` line also shows how long that message waited (illustrative):

```
2026/10/06 16:10:02.103000 INFO random read delay on max=400ms
2026/10/06 16:10:02.104000 INFO SENT random=K7QXJ3M2WZP4NB6TR5DYH2VFGA ts=2026-10-06T16:10:02.104+06:00
2026/10/06 16:10:02.331000 INFO RECV random=K7QXJ3M2WZP4NB6TR5DYH2VFGA ts=2026-10-06T16:10:02.104+06:00 delay=226ms
```

A message that is not valid JSON is logged as `WARN RECV err=... body=...` and still acknowledged.

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
