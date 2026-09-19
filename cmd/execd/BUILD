load("//tools:defs.bzl", "go_binary", "go_library", "go_test")

package(
    default_applicable_licenses = ["//:license"],
    licenses = ["notice"],
)

go_library(
    name = "lib",
    srcs = [
        "exec.go",
        "file.go",
        "frame.go",
        "git.go",
        "ledger.go",
        "main.go",
        "patch.go",
        "pty.go",
        "root.go",
        "serve.go",
        "watch.go",
    ],
    stateify = False,
    deps = [
        "@com_github_creack_pty//:go_default_library",
        "@com_github_zap_proto_go//:go_default_library",
        "@org_golang_x_sys//unix:go_default_library",
    ],
)

# The sources are the library's, so nogo analyses them there.
go_binary(
    name = "execd",
    embed = [":lib"],
    nogo = False,
    pure = True,
    visibility = ["//visibility:public"],
)

go_test(
    name = "execd_test",
    size = "medium",
    srcs = [
        "frame_test.go",
        "ledger_test.go",
        "patch_test.go",
        "root_test.go",
        "serve_test.go",
    ],
    library = ":lib",
    deps = ["@org_golang_x_sys//unix:go_default_library"],
)
