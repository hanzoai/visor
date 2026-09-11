load("//tools:defs.bzl", "go_library", "go_test")

package(
    default_applicable_licenses = ["//:license"],
    licenses = ["notice"],
)

go_library(
    name = "config",
    srcs = [
        "cgo_disabled.go",
        "cgo_enabled.go",
        "config.go",
        "config_bundles.go",
        "flags.go",
        "flags_graveyard.go",
        "zap.go",
    ],
    visibility = ["//:sandbox"],
    deps = [
        "//pkg/log",
        "//pkg/refs",
        "//pkg/sentry/devices/nvproxy/nvconf",
        "//pkg/sync",
        "//runsc/flag",
        "//runsc/version",
        "@com_github_zap_proto_go//:go",
    ],
)

go_test(
    name = "config_test",
    size = "small",
    srcs = [
        "config_test.go",
        "flags_test.go",
    ],
    library = ":config",
    deps = [
        "//runsc/flag",
        "@com_github_google_go_cmp//cmp:go_default_library",
    ],
)
