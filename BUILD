load("@gazelle//:def.bzl", "gazelle")
load("@rules_go//go:def.bzl", "go_binary", "go_library")

gazelle(name = "gazelle")

go_library(
    name = "rbp-control-i2c-multiplexer_lib",
    srcs = ["main.go"],
    importpath = "all4dich/rbp-control-i2c-multiplexer",
    visibility = ["//visibility:private"],
    deps = [
        "@com_github_prometheus_client_golang//prometheus:go_default_library",
        "@com_github_prometheus_client_golang//prometheus/promauto:go_default_library",
        "@com_github_prometheus_client_golang//prometheus/promhttp:go_default_library",
        "@io_periph_x_conn_v3//i2c:go_default_library",
        "@io_periph_x_conn_v3//i2c/i2creg:go_default_library",
        "@io_periph_x_host_v3//:go_default_library",
    ],
)

go_binary(
    name = "rbp-control-i2c-multiplexer",
    embed = [":rbp-control-i2c-multiplexer_lib"],
    visibility = ["//visibility:public"],
)
