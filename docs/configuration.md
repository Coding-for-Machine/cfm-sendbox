# Configuration

Piston provides many different configuration options to tweak Piston to meet your needs.

Configuration is specified through environment variables, prefixed with `PISTON_`.

## Log Level

```yaml
key: CFM_LOG_LEVEL
default: INFO
```

Level of log output to provide.

One of `DEBUG`, `INFO`, `WARN`, `ERROR` or `NONE`

## Bind Address

```yaml
key: CFM_BIND_ADDRESS
default: 0.0.0.0:2000
```

Port and IP address to bind the Piston API to.

<!-- prettier-ignore -->
!!! warning
    Changing this value is not recommended.

    This changes the bind address inside the container, and thus serves no purpose when running in a container

## Data Directory

```yaml
key: CFM_DATA_DIRECTORY
default: /piston
```

Absolute path to piston related data, including packages and job contexts.

<!-- prettier-ignore -->
!!! warning
    Changing this value is not recommended.

    Some packages require absolute paths on disk at build time.
    Due to this, some packages may break when changing this parameter.

## Runner GID/UID range

```yaml
key:
    - CFM_RUNNER_UID_MIN
    - CFM_RUNNER_UID_MAX
    - CFM_RUNNER_GID_MIN
    - CFM_RUNNER_GID_MAX
default:
    - 1001
    - 1500
    - 1001
    - 1500
```

UID and GID ranges to use when executing jobs.

<!-- prettier-ignore -->
!!! warning
    Changing this value is not recommended.

    The piston container creates 500 users and groups by default, and reserves user/group 1000 for running the API.
    Any processes run by these users will be killed when cleaning up a job.

## Disable Networking

```yaml
key: CFM_DISABLE_NETWORKING
default: true
```

Disallows access to `socket` syscalls, effectively disabling networking for jobs run by piston.

## Max Process Count

```yaml
key: CFM_MAX_PROCESS_COUNT
default: 64
```

Maximum number of processes allowed to to have open for a job.

Resists against exhausting the process table, causing a full system lockup.

## Output Max Size

```yaml
key: CFM_OUTPUT_MAX_SIZE
default: 1024
```

Maximum size of stdio buffers for each job.

Resist against run-away output which could lead to memory exhaustion.

## Max Open Files

```yaml
key: CFM_MAX_OPEN_FILES
default: 64
```

Maximum number of open files at a given time by a job.

Resists against writing many smaller files to exhaust inodes.

## Max File Size

```yaml
key: CFM_MAX_FILE_SIZE
default: 10000000 #10MB
```

Maximum size for a singular file written to disk.

Resists against large file writes to exhaust disk space.

## Compile/Run timeouts

```yaml
key:
  - CFM_COMPILE_TIMEOUT
default: 10000

key:
  - CFM_RUN_TIMEOUT
default: 3000
```

The maximum time that is allowed to be taken by a stage in milliseconds. This is the wall-time of the stage. The time that the CPU does not spend working on the stage (e.g, due to context switches or IO) is counted.

## Compile/Run CPU-Time

```yaml
key:
  - CFM_COMPILE_CPU_TIME
default: 10000

key:
  - CFM_RUN_CPU_TIME
default: 3000
```

The maximum CPU-time that is allowed to be consumed by a stage in milliseconds. The time that the CPU does not spend working on the stage (e.g, IO and context switches) is not counted. This option is typically used in algorithm contests.

## Compile/Run memory limits

```yaml
key:
    - CFM_COMPILE_MEMORY_LIMIT
    - CFM_RUN_MEMORY_LIMIT
default: -1
```

Maximum memory allowed by a stage in bytes.
Use -1 for unlimited memory usage.

Useful for running memory-limited contests.

## Repository URL

Comma separated list of package indexes, merged (first wins). The second one is the upstream Piston index with ready-made packages (x86_64). Legacy `PISTON_*` variable names are still accepted for every option.

```yaml
key: CFM_REPO_URL
default: https://github.com/Coding-for-Machine/cfm-sendbox/releases/download/pkgs/index,https://github.com/engineer-man/piston/releases/download/pkgs/index
```

URL for repository index, where packages will be downloaded from.

## Maximum Concurrent Jobs

```yaml
key: CFM_MAX_CONCURRENT_JOBS
default: 64
```

Maximum number of jobs to run concurrently.

## Limit overrides

```yaml
key: CFM_LIMIT_OVERRIDES
default: {}
```

Per-language overrides/exceptions for the each of `max_process_count`, `max_open_files`, `max_file_size`,
`compile_memory_limit`, `run_memory_limit`, `compile_timeout`, `run_timeout`, `compile_cpu_time`, `run_cpu_time`, `output_max_size`. Defined as follows:

```
CFM_LIMIT_OVERRIDES={"c++":{"max_process_count":128}}
```

This will give `c++` a max_process_count of 128 regardless of the configuration.

## API keys

```yaml
key: CFM_API_KEYS
default: (empty - authentication disabled)
```

Comma separated list of accepted API keys. When set, every `/api/v2/*` request needs one of them
(`/` and `/healthz` stay open). The key is read from, in this order:

- `Authorization: Bearer <key>`
- `X-API-Key: <key>`
- `?api_key=<key>` query parameter (for WebSocket clients that cannot set headers)

```sh
# server
CFM_API_KEYS="key1,key2" docker compose -f docker-compose.yaml up -d
# curl
curl -H "Authorization: Bearer key1" localhost:2000/api/v2/runtimes
# CLI
export CFM_API_KEY=key1          # or: ./cfm -k key1 runtimes
./cfm run python t.py
```

Generate a key with `openssl rand -hex 32`. Rotate keys by listing the old and the new one, then removing the old one.
Use HTTPS (reverse proxy) when the API is reachable from the internet.
