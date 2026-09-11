load("//tools:defs.bzl", "go_library", "go_test")

package(
    default_applicable_licenses = ["//:license"],
    licenses = ["notice"],
)

go_library(
    name = "urpc",
    srcs = [
        "urpc.go",
        "zap.go",
    ],
    visibility = ["//:sandbox"],
    deps = [
        "//pkg/fd",
        "//pkg/log",
        "//pkg/sync",
        "//pkg/unet",
        "@com_github_zap_proto_go//:go",
    ],
)

go_test(
    name = "urpc_test",
    size = "small",
    srcs = ["urpc_test.go"],
    library = ":urpc",
    deps = ["//pkg/unet"],
)
