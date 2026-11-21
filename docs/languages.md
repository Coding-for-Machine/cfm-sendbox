# Supported languages & versions

Snapshot: 2026-11-21. Versions were collected from upstream release pages / endoflife.date while preparing this
repository; build scripts for the new versions have **not** been built in CI yet - run `./cfm build-pkg <lang> <version>`
to verify a package before publishing it. Status labels are informal; check endoflife.date for exact support windows.

## Updated packages

| Language | Version | Status |
|---|---|---|
| `python` | 3.15.2 | current |
| `python` | 3.14.10 | stable |
| `python` | 3.13.18 | stable |
| `python` | 3.12.16 | security-only |
| `python` | 3.11.18 | security-only |
| `node` | 26.13.1 | current (LTS from Oct 2026) |
| `node` | 24.22.3 | LTS |
| `node` | 22.23.5 | LTS (maintenance) |
| `go` | 1.27.4 | current |
| `go` | 1.26.11 | supported |
| `java` | 25.0.5 | LTS |
| `java` | 21.0.13 | LTS |
| `java` | 17.0.21 | LTS |
| `kotlin` | 2.4.21 | current |
| `scala` | 3.9.0 | LTS |
| `scala` | 3.3.9 | LTS |
| `groovy` | 5.1.3 | current |
| `groovy` | 4.0.33 | stable |
| `rust` | 1.100.0 | stable |
| `php` | 8.5.12 | current |
| `php` | 8.4.27 | supported |
| `php` | 8.3.36 | security-only |
| `ruby` | 4.0.10 | current |
| `ruby` | 3.4.13 | stable |
| `perl` | 5.44.0 | current |
| `perl` | 5.42.3 | stable |
| `rscript` | 4.6.1 | current |
| `lua` | 5.5.1 | current |
| `bash` | 5.3.0 | current |
| `pwsh` | 7.6.10 | LTS |
| `sqlite3` | 3.54.1 | current |
| `erlang` | 29.1.2 | current |
| `elixir` | 1.20.5 | current |
| `haskell` | 9.14.1 | current |
| `gcc` | 16.2.0 | current |
| `gcc` | 15.3.0 | stable |
| `gcc` | 14.4.0 | stable |
| `julia` | 1.13.3 | current |
| `julia` | 1.10.13 | LTS |
| `swift` | 6.4.3 | current |
| `zig` | 0.16.0 | current |
| `dart` | 3.14.1 | current |
| `dotnet` | 10.0.15 | LTS |
| `deno` | 2.9.8 | current |
| `bun` | 1.4.4 | current |
| `typescript` | 7.0.2 | current |
| `typescript` | 6.0.2 | previous |

## Full inventory (all package directories)

Older versions are kept so existing clients keep working; prefer the versions above.

| Language | Versions |
|---|---|
| `MATL` | 22.7.4, 22.5.0 |
| `bash` | 5.3.0, 5.2.0, 5.1.0 |
| `befunge93` | 0.2.0 |
| `bqn` | 1.0.0 |
| `brachylog` | 1.0.0 |
| `brainfuck` | 2.7.3 |
| `bun` | 1.4.4 |
| `cjam` | 0.6.5 |
| `clojure` | 1.10.3 |
| `cobol` | 3.1.2 |
| `coffeescript` | 2.5.1 |
| `cow` | 1.0.0 |
| `crystal` | 1.9.2 |
| `dart` | 3.14.1, 3.0.1, 2.19.6, 2.12.1 |
| `dash` | 0.5.11 |
| `deno` | 2.9.8, 1.32.3, 1.16.2, 1.7.5 |
| `dotnet` | 10.0.15, 5.0.201 |
| `dragon` | 1.9.8 |
| `elixir` | 1.20.5, 1.11.3 |
| `emacs` | 27.1.0 |
| `emojicode` | 1.0.2 |
| `erlang` | 29.1.2, 23.0.0 |
| `file` | 0.0.1 |
| `forte` | 1.0.0 |
| `forth` | 0.7.3 |
| `freebasic` | 1.9.0, 1.8.0 |
| `gawk` | 5.1.0 |
| `gcc` | 16.2.0, 15.3.0, 14.4.0, 10.2.0 |
| `go` | 1.27.4, 1.26.11, 1.16.2 |
| `golfscript` | 1.0.0 |
| `groovy` | 5.1.3, 4.0.33, 3.0.7 |
| `haskell` | 9.14.1, 9.0.1 |
| `husk` | 1.0.0 |
| `iverilog` | 11.0.0 |
| `japt` | 2.0.0 |
| `java` | 25.0.5, 21.0.13, 17.0.21, 15.0.2 |
| `jelly` | 0.1.31 |
| `julia` | 1.13.3, 1.10.13, 1.8.5, 1.6.1, 1.5.4 |
| `kotlin` | 2.4.21, 1.8.20, 1.4.31 |
| `lisp` | 2.1.2 |
| `llvm_ir` | 12.0.1 |
| `lolcode` | 0.11.2 |
| `lua` | 5.5.1, 5.4.4, 5.4.2 |
| `mono` | 6.12.0 |
| `nasm` | 2.15.5 |
| `nim` | 1.6.2, 1.4.4 |
| `node` | 26.13.1, 24.22.3, 22.23.5, 20.11.1, 18.15.0, 16.3.0, 15.10.0 |
| `ocaml` | 4.12.0 |
| `octave` | 8.1.0, 6.2.0 |
| `osabie` | 1.0.1 |
| `paradoc` | 0.6.0 |
| `pascal` | 3.2.2, 3.2.0 |
| `perl` | 5.44.0, 5.42.3, 5.36.0, 5.26.1 |
| `php` | 8.5.12, 8.4.27, 8.3.36, 8.2.3, 8.0.2 |
| `ponylang` | 0.39.0 |
| `prolog` | 8.2.4 |
| `pure` | 0.68.0 |
| `pwsh` | 7.6.10, 7.1.4 |
| `pyth` | 1.0.0 |
| `python` | 3.15.2, 3.14.10, 3.13.18, 3.12.16, 3.12.0, 3.11.18, 3.11.0, 3.10.0-alpha.7, 3.10.0, 3.9.4, 3.9.1, 3.5.10, 2.7.18 |
| `racket` | 8.3.0 |
| `raku` | 6.100.0 |
| `retina` | 1.2.0 |
| `rockstar` | 1.0.0 |
| `rscript` | 4.6.1, 4.1.1 |
| `ruby` | 4.0.10, 3.4.13, 3.0.1, 2.5.1 |
| `rust` | 1.100.0, 1.68.2, 1.65.0, 1.63.0, 1.62.0, 1.56.1, 1.50.0 |
| `samarium` | 0.3.1 |
| `scala` | 3.9.0, 3.3.9, 3.2.2, 3.0.0 |
| `smalltalk` | 3.2.3 |
| `sqlite3` | 3.54.1, 3.36.0 |
| `swift` | 6.4.3, 5.3.3 |
| `typescript` | 7.0.2, 6.0.2, 5.0.3, 4.2.3 |
| `vlang` | 0.3.3, 0.1.13 |
| `vyxal` | 2.4.1 |
| `yeethon` | 3.10.0 |
| `zig` | 0.16.0, 0.10.1, 0.9.1, 0.8.0, 0.7.1 |
