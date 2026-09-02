# cookeR

A standalone R version, environment, and package manager. `cooker` is published together with a thin R package (`cookeR`) that wraps it for use inside an R session.

Installs prebuilt R runtimes (relocatable, CI-built R releases), manages isolated per-project library environments, and installs R packages into them.

## Architecture

- **Go CLI** (`main.go`, `internal/rversion`, `internal/appdir`) handles download/verify/extract/version-management pipeline for R runtimes. Usable standalone from any shell, CI, or embedded in other tools.
- **R package** thin wrapper that downloads/caches the `cooker` binary and calls it via `system2()`/`processx`, parsing its `--json` output.

## CLI usage (current)

```
cooker r install <version>       Install an R version from r-portable
cooker r list [--available]      List installed (or available) R versions
cooker r uninstall <version>     Remove an installed R version
cooker r path <version>          Print the Rscript path for an installed version
cooker doctor                    Report cookeR's environment status
```

R versions install under an OS-appropriate default data directory (override with the`COOKER_INSTALL_DIR` environment variable).

## Development

```
go build ./...
go test ./...
```

## License

MIT (see `LICENSE`). R runtimes installed via `cooker` remain licensed under GPL-2/GPL-3 by the R Foundation.
