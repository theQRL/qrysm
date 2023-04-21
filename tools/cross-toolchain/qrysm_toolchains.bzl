def _qrysm_toolchains_impl(ctx):
    ctx.template(
        "BUILD.bazel",
        ctx.attr._build_tpl,
    )
    ctx.template(
        "cc_toolchain_config_linux_arm64.bzl",
        ctx.attr._cc_toolchain_config_linux_arm_tpl,
    )
    ctx.template(
        "cc_toolchain_config_osx.bzl",
        ctx.attr._cc_toolchain_config_osx_tpl,
    )
    ctx.template(
        "cc_toolchain_config_windows.bzl",
        ctx.attr._cc_toolchain_config_windows_tpl,
    )

qrysm_toolchains = repository_rule(
    implementation = _qrysm_toolchains_impl,
    attrs = {
        "_build_tpl": attr.label(
            default = "@qrysm//tools/cross-toolchain:cc_toolchain.BUILD.bazel.tpl",
        ),
        "_cc_toolchain_config_linux_arm_tpl": attr.label(
            default = "@qrysm//tools/cross-toolchain:cc_toolchain_config_linux_arm64.bzl.tpl",
        ),
        "_cc_toolchain_config_osx_tpl": attr.label(
            default = "@qrysm//tools/cross-toolchain:cc_toolchain_config_osx.bzl.tpl",
        ),
        "_cc_toolchain_config_windows_tpl": attr.label(
            default = "@qrysm//tools/cross-toolchain:cc_toolchain_config_windows.bzl.tpl",
        ),
    },
    doc = "Configures Qrysm custom toolchains for cross compilation and remote build execution.",
)

def configure_qrysm_toolchains():
    qrysm_toolchains(name = "qrysm_toolchains")
    native.register_toolchains("@qrysm_toolchains//:cc-toolchain-multiarch")
