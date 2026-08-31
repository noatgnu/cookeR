#' Install an R version
#'
#' Downloads, verifies, and installs the given R version via `cooker r install`.
#'
#' @param version Character. The R version to install, e.g. `"4.4.2"`.
#' @return Invisibly, the CLI's output lines.
#' @export
install_r <- function(version) {
  .cooker_run(c("r", "install", version))
}

#' List R versions
#'
#' @param available If `TRUE`, list versions available to install instead of installed versions. Default `FALSE`.
#' @return A character vector of version strings.
#' @export
list_r_versions <- function(available = FALSE) {
  args <- c("r", "list", "--json")
  if (available) {
    args <- c(args, "--available")
  }
  result <- .cooker_run_json(args)
  if (is.data.frame(result)) {
    return(result$version)
  }
  unlist(result)
}

#' Remove an installed R version
#'
#' @param version Character. The R version to uninstall.
#' @return Invisibly, the CLI's output lines.
#' @export
uninstall_r <- function(version) {
  .cooker_run(c("r", "uninstall", version))
}

#' Get the Rscript path for an installed R version
#'
#' @param version Character. The installed R version.
#' @return A string path to the version's `Rscript` binary.
#' @export
r_path <- function(version) {
  result <- .cooker_run(c("r", "path", version))
  trimws(result[[1]])
}
