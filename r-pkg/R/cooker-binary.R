#' Locate the cooker binary
#'
#' Resolves the path to the `cooker` command-line tool via `COOKER_BIN`, the package cache, or `PATH`, in that order.
#'
#' @return A string path to the `cooker` executable.
#' @export
cooker_binary_path <- function() {
  env_override <- Sys.getenv("COOKER_BIN", unset = NA)
  if (!is.na(env_override) && nzchar(env_override) && file.exists(env_override)) {
    return(env_override)
  }

  cache_dir <- tools::R_user_dir("cookeR", which = "cache")
  binary_name <- if (.Platform$OS.type == "windows") "cooker.exe" else "cooker"
  cached_path <- file.path(cache_dir, binary_name)
  if (file.exists(cached_path)) {
    return(cached_path)
  }

  on_path <- Sys.which("cooker")
  if (nzchar(on_path)) {
    return(unname(on_path))
  }

  stop(
    "cooker binary not found. Set the COOKER_BIN environment variable to its path, ",
    "install it on your PATH, or run cookeR::cooker_install() once binary distribution ",
    "is available.",
    call. = FALSE
  )
}

#' Download and cache the cooker binary for the current platform
#'
#' @description Not yet implemented -- cookeR has no published releases to download from yet.
#'
#' @export
cooker_install <- function() {
  stop("cooker_install() is not yet implemented -- cookeR has no published releases yet.", call. = FALSE)
}

#' Run a cooker subcommand and parse its JSON output
#'
#' @param args Character vector of CLI arguments, e.g. `c("r", "list", "--json")`.
#' @return The parsed JSON result (typically a data.frame or list).
#' @keywords internal
.cooker_run_json <- function(args) {
  bin <- cooker_binary_path()
  result <- system2(bin, args, stdout = TRUE, stderr = TRUE)
  status <- attr(result, "status")
  if (!is.null(status) && status != 0) {
    stop("cooker ", paste(args, collapse = " "), " failed:\n", paste(result, collapse = "\n"), call. = FALSE)
  }
  jsonlite::fromJSON(paste(result, collapse = "\n"))
}

#' Run a cooker subcommand without expecting JSON output
#'
#' @param args Character vector of CLI arguments.
#' @return Invisibly, the character vector of combined stdout/stderr lines.
#' @keywords internal
.cooker_run <- function(args) {
  bin <- cooker_binary_path()
  result <- system2(bin, args, stdout = TRUE, stderr = TRUE)
  status <- attr(result, "status")
  if (!is.null(status) && status != 0) {
    stop("cooker ", paste(args, collapse = " "), " failed:\n", paste(result, collapse = "\n"), call. = FALSE)
  }
  invisible(result)
}
