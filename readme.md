<h1 align="center">cfm-sendbox</h1>

<p align="center">
Code execution sandbox for <a href="https://cfmcontest.uz">cfmcontest.uz</a> — a fully Go implementation of the
<a href="https://github.com/engineer-man/piston">Piston</a> v3 stack: API (Fiber v3), CLI and package repository server.
No Node.js, no Python tooling.
</p>

# Layout

| Path | What |
| --- | --- |
| `cmd/sendbox-api` | API server (Go, [Fiber v3](https://docs.gofiber.io/) + [contrib/v3/websocket](https://docs.gofiber.io/contrib/websocket/)) |
| `cmd/sendbox` | CLI (Go, stdlib only; built as `bin/cfm`) |
| `cmd/sendbox-repo` | Package index/file server (Go) |
| `internal/` | API internals (`api`, `config`, `job`, `pkgman`, `runtime`, `globals`) and CLI internals (`cli/*`) |
| `packages/` | Language package definitions (`build.sh`, `metadata.json`, `run`, ...) |
| `docker/api`, `repo/`, `builder/` | Dockerfiles and package builder scripts |
| `docs/` | Documentation, incl. [supported languages & versions](docs/languages.md) |

Go module: `github.com/Coding-for-Machine/cfm-sendbox` (Go 1.26+ because of Fiber v3).

# Getting Started

### Host system dependencies

-   Docker (+ Docker Compose v2)
-   Go 1.26+ (only to build the CLI; the `./cfm` script falls back to a `golang` Docker image)
-   cgroup v2 enabled, cgroup v1 disabled

### Install

```sh
git clone https://github.com/Coding-for-Machine/cfm-sendbox
cd cfm-sendbox
go mod tidy                 # resolves Fiber v3 / contrib websocket, writes go.sum
go get -u github.com/gofiber/fiber/v3
go get -u github.com/gofiber/contrib/v3/websocket   # (optional: upgrade to latest)

docker compose up -d --build        # API on :2000, no runtimes installed yet
./cfm ppman list                    # packages from the cfm index + upstream Piston index
./cfm ppman install python=3.12.0   # ready-made package (prebuilt, x86_64)
./cfm run python hello.py
./cfm ppman spec public.pps         # install everything listed in public.pps
./cfm runtimes
```

Handy targets: `make build | test | lint | tidy`, `./cfm help`.

### Only the API container

```sh
docker build -f docker/api/Dockerfile -t cfm-sendbox .
docker run --privileged -v $PWD/data/cfm:/cfm -dit -p 2000:2000 --name sendbox_api cfm-sendbox
```

<br>

# Usage

### CLI

The CLI is the main tool used for installing packages within cfm-sendbox, but also supports running code.

The CLI is a single static Go binary (no Node.js). `./cfm <args>` builds it on first use (`bin/cfm`, needs Go or Docker) and forwards the arguments; or run `make cli` and use `bin/cfm` directly.

```sh
# List all available packages
./cfm ppman list

# Install latest python
./cfm ppman install python

# Install specific version of python
./cfm ppman install python=3.9.4

# Run a python script using the latest version
echo 'print("Hello world!")' > test.py
./cfm run python test.py

# Run a python script using a specific version
echo 'print("Hello world!")' > test.py
./cfm run python test.py -l 3.9.4
./cfm run python test.py -l 3.x
./cfm run python test.py -l 3
```

If you are operating on a remote machine, add the `-u` flag like so:

```sh
./cfm -u http://cfm.server:2000 ppman list
```

If the server has API keys enabled (`CFM_API_KEYS`), give the CLI a key with `-k` or the `CFM_API_KEY` environment variable:

```sh
export CFM_API_KEY=your-key
./cfm ppman list

# or per command
./cfm -u http://cfm.server:2000 -k your-key run python test.py
```

### API

The container exposes an API on port 2000 by default.
This is used by the CLI to carry out running jobs and package management.

#### Authentication (API key)

Set one or more keys on the server (comma separated) in `.env` (see `.env.example`) or the environment:

```env
CFM_API_KEYS=key1,key2
```

Generate a key with `openssl rand -hex 32`. When keys are set, every `/api/v2/*` request must carry one of them;
`/` and `/healthz` stay open. Without keys the API is open (like upstream Piston). Send the key as:

-   `Authorization: Bearer <key>` (recommended)
-   `X-API-Key: <key>`
-   `?api_key=<key>` query parameter (for WebSocket clients that cannot set headers)

```sh
curl -H "Authorization: Bearer key1" http://cfm.server:2000/api/v2/runtimes

curl -X POST http://cfm.server:2000/api/v2/execute \
  -H "Authorization: Bearer key1" -H "Content-Type: application/json" \
  -d '{"language":"python","version":"*","files":[{"content":"print(1+1)"}]}'
```

WebSocket: `ws://cfm.server:2000/api/v2/connect?api_key=key1`.
A wrong or missing key returns `401 {"message":"Invalid or missing API key"}`. Use HTTPS (reverse proxy) when the API is reachable from the internet.

#### Runtimes Endpoint

`GET /api/v2/runtimes`
This endpoint will return the supported languages along with the current version and aliases. To execute
code for a particular language using the `/api/v2/execute` endpoint, either the name or one of the aliases must
be provided, along with the version.
Multiple versions of the same language may be present at the same time, and may be selected when running a job.

```json
HTTP/1.1 200 OK
Content-Type: application/json

[
    {
        "language": "bash",
        "version": "5.1.0",
        "aliases": [
            "sh"
        ]
    },
    {
        "language": "brainfuck",
        "version": "2.7.3",
        "aliases": [
            "bf"
        ]
    },
    ...
]
```

#### Execute Endpoint

`POST /api/v2/execute`
This endpoint requests execution of some arbitrary code.

-   `language` (**required**) The language to use for execution, must be a string and must be installed.
-   `version` (**required**) The version of the language to use for execution, must be a string containing a SemVer selector for the version or the specific version number to use.
-   `files` (**required**) An array of files containing code or other data that should be used for execution. The first file in this array is considered the main file.
-   `files[].name` (_optional_) The name of the file to upload, must be a string containing no path or left out.
-   `files[].content` (**required**) The content of the files to upload, must be a string containing text to write.
-   `files[].encoding` (_optional_) The encoding scheme used for the file content. One of `base64`, `hex` or `utf8`. Defaults to `utf8`.
-   `stdin` (_optional_) The text to pass as stdin to the program. Must be a string or left out. Defaults to blank string.
-   `args` (_optional_) The arguments to pass to the program. Must be an array or left out. Defaults to `[]`.
-   `compile_timeout` (_optional_) The maximum wall-time allowed for the compile stage to finish before bailing out in milliseconds. Must be a number or left out. Defaults to `10000` (10 seconds).
-   `run_timeout` (_optional_) The maximum wall-time allowed for the run stage to finish before bailing out in milliseconds. Must be a number or left out. Defaults to `3000` (3 seconds).
-   `compile_cpu_time` (_optional_) The maximum CPU-time allowed for the compile stage to finish before bailing out in milliseconds. Must be a number or left out. Defaults to `10000` (10 seconds).
-   `run_cpu_time` (_optional_) The maximum CPU-time allowed for the run stage to finish before bailing out in milliseconds. Must be a number or left out. Defaults to `3000` (3 seconds).
-   `compile_memory_limit` (_optional_) The maximum amount of memory the compile stage is allowed to use in bytes. Must be a number or left out. Defaults to `-1` (no limit)
-   `run_memory_limit` (_optional_) The maximum amount of memory the run stage is allowed to use in bytes. Must be a number or left out. Defaults to `-1` (no limit)

```json
{
    "language": "js",
    "version": "15.10.0",
    "files": [
        {
            "name": "my_cool_code.js",
            "content": "console.log(process.argv)"
        }
    ],
    "stdin": "",
    "args": ["1", "2", "3"],
    "compile_timeout": 10000,
    "run_timeout": 3000,
    "compile_cpu_time": 10000,
    "run_cpu_time": 3000,
    "compile_memory_limit": -1,
    "run_memory_limit": -1
}
```

A typical response upon successful execution will contain 1 or 2 keys `run` and `compile`.
`compile` will only be present if the language requested requires a compile stage.

Each of these keys has an identical structure, containing both a `stdout` and `stderr` key, which is a string containing the text outputted during the stage into each buffer.
It also contains the `code` and `signal` which was returned from each process. It also includes a nullable human-readable `message` which is a description of why a stage has failed and a two-letter `status` that is either:

-   `RE` for runtime error
-   `SG` for dying on a signal
-   `TO` for timeout (either via `timeout` or `cpu_time`)
-   `OL` for stdout length exceeded
-   `EL` for stderr length exceeded
-   `XX` for internal error

```json
HTTP/1.1 200 OK
Content-Type: application/json

{
    "language": "js",
    "version": "15.10.0",
    "run": {
        "stdout": "[\n  '/cfm/packages/node/15.10.0/bin/node',\n  '/cfm/jobs/9501b09d-0105-496b-b61a-e5148cf66384/my_cool_code.js',\n  '1',\n  '2',\n  '3'\n]\n",
        "stderr": "",
        "output": "[\n  '/cfm/packages/node/15.10.0/bin/node',\n  '/cfm/jobs/9501b09d-0105-496b-b61a-e5148cf66384/my_cool_code.js',\n  '1',\n  '2',\n  '3'\n]\n",
        "code": 0,
        "signal": null,
        "message": null,
        "status": null,
        "cpu_time": 8,
        "wall_time": 154,
        "memory": 1160000
    }
}
```

If a problem exists with the request, a `400` status code is returned and the reason in the `message` key.

```json
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
    "message": "html-5.0.0 runtime is unknown"
}
```

#### Interactive execution endpoint (not available through the public API)

To interact with running processes in real time, you can establish a WebSocket connection at `/api/v2/connect`. This allows you to both receive output and send input to active processes.

Each message is structured as a JSON object with a `type` key, which indicates the action to perform. Below is a list of message types, their directions, and descriptions:

-   **init** (client -> server): Initializes a job with the same parameters as the `/execute` endpoint, except that stdin is discarded.
-   **runtime** (server -> client): Provides details on the runtime environment, including the version and language.
-   **stage** (server -> client): Indicates the current execution stage, either "compile" or "run."
-   **data** (server <-> client): Exchanges data between the client and server, such as stdin, stdout, or stderr streams.
-   **signal** (client -> server): Sends a signal (e.g., for termination) to the running process, whether it's in the "compile" or "run" stage.
-   **exit** (server -> client): Signals the end of a stage, along with the exit code or signal.
-   **error** (server -> client): Reports an error, typically right before the WebSocket is closed.

An example of this endpoint in use is depicted below (**<** = client to server, **>** = server to client)

1. Client establishes WebSocket connection to `/api/v2/connect`
2. **<** `{"type":"init", "language":"bash", "version":"*", "files":[{"content": "cat"}]}`
3. **>** `{"type":"runtime","language": "bash", "version": "5.1.0"}`
4. **>** `{"type":"stage", "stage":"run"}`
5. **<** `{"type":"data", "stream":"stdin", "data":"Hello World!"}`
6. **>** `{"type":"data", "stream":"stdout", "data":"Hello World!"}`
7. _time passes_
8. **>** `{"type":"exit", "stage":"run", "code":null, "signal": "SIGKILL"}`

Errors may return status codes as follows:

-   **4000: Already Initialized**: Sent when a second `init` command is issued.
-   **4001: Initialization Timeout**: No `init` command was sent within 1 second of connection.
-   **4002: Notified Error**: A fatal error occurred, and an `error` packet was transmitted.
-   **4003: Not yet Initialized**: A non-`init` command was sent without a job context.
-   **4004: Can only write to stdin**: The client attempted to write to a stream other than stdin.
-   **4005: Invalid Signal**: An invalid signal was sent in a `signal` packet.

<br>

# Supported Languages

`awk`,
`bash`,
`befunge93`,
`brachylog`,
`brainfuck`,
`bqn`,
`c`,
`c++`,
`cjam`,
`clojure`,
`cobol`,
`coffeescript`,
`cow`,
`crystal`,
`csharp`,
`csharp.net`,
`d`,
`dart`,
`dash`,
`dragon`,
`elixir`,
`emacs`,
`emojicode`,
`erlang`,
`file`,
`forte`,
`forth`,
`fortran`,
`freebasic`,
`fsharp.net`,
`fsi`,
`go`,
`golfscript`,
`groovy`,
`haskell`,
`husk`,
`iverilog`,
`japt`,
`java`,
`javascript`,
`jelly`,
`julia`,
`kotlin`,
`lisp`,
`llvm_ir`,
`lolcode`,
`lua`,
`matl`,
`nasm`,
`nasm64`,
`nim`,
`ocaml`,
`octave`,
`osabie`,
`paradoc`,
`pascal`,
`perl`,
`php`,
`ponylang`,
`powershell`,
`prolog`,
`pure`,
`pyth`,
`python`,
`python2`,
`racket`,
`raku`,
`retina`,
`rockstar`,
`rscript`,
`ruby`,
`rust`,
`samarium`,
`scala`,
`smalltalk`,
`sqlite3`,
`swift`,
`typescript`,
`basic`,
`basic.net`,
`vlang`,
`vyxal`,
`yeethon`,
`zig`,

<br>

# Principle of Operation

cfm-sendbox (like Piston) uses [Isolate](https://www.ucw.cz/moe/isolate.1.html) inside Docker as the primary mechanism for sandboxing. There is an API within the container written in Go (Fiber)
which takes in execution requests and executes them within the container safely.
High level, the API writes any source code and executes it inside an Isolate sandbox.
The source file is either ran or compiled and ran (in the case of languages like c, c++, c#, go, etc.).

<br>

# Security

cfm-sendbox uses Isolate, which makes use of Linux namespaces, chroot, multiple unprivileged users, and cgroup for sandboxing and resource limiting. Code execution submissions shall not be aware of each other, shall not affect each other and shall not affect the underlying host system. This is ensured through multiple steps including:

-   Disabling outgoing network interaction by default
-   Capping max processes at 256 by default (resists `:(){ :|: &}:;`, `while True: os.fork()`, etc.)
-   Capping max files at 2048 (resists various file based attacks)
-   Cleaning up all temp space after each execution (resists out of drive space attacks)
-   Running each submission as a different unprivileged user
-   Running each submission with its own isolated Linux namespaces
-   Capping runtime execution at 3 seconds by default (CPU-time and wall-time)
-   Capping the peak memory that all the submission's processes can use
-   Capping stdout to 1024 characters by default (resists yes/no bombs and runaway output)
-   SIGKILLing misbehaving code

<br>

# License

cfm-sendbox is licensed under the MIT license (based on the MIT-licensed [Piston](https://github.com/engineer-man/piston)).

