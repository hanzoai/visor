load("//tools:defs.bzl", "go_library")

package(default_applicable_licenses = ["//:license"])

licenses(["notice"])

go_library(
    name = "checkpoint",
    srcs = [
        "checkpoint.go",
        "zap.go",
    ],
    visibility = ["//pkg/sentry:internal"],
    deps = [
        "@com_github_zap_proto_go//:go",
    ],
)
