load("//tools:defs.bzl", "go_binary")

package(default_applicable_licenses = ["//:license"])

licenses(["notice"])

go_binary(
    name = "zap",
    srcs = ["zap.go"],
    visibility = ["//:sandbox"],
    deps = [
        "//pkg/sentry/control",
        "//pkg/sentry/state/stateipc",
        "//pkg/urpc",
        "//runsc/boot",
        "//runsc/container",
        "@com_github_zap_proto_zip//:zip",
    ],
)
