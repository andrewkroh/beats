"""
Defines custom build rules that allow to use Go text/template package.
"""

def _gotemplate_impl(ctx):
    out_file = ctx.actions.declare_file(ctx.label.name)
    args = ctx.actions.args()
    args.add_all([
        "-i",
        ctx.file.template,
        "-o",
        out_file,
    ])

    ctx.actions.run(
        outputs = [out_file],
        inputs = [ctx.file.template],
        executable = ctx.executable._gotemplate,
        arguments = [args],
        mnemonic = "GoTemplate",
        progress_message = "Assembling %s" % out_file.short_path,
    )
    return [DefaultInfo(
        files = depset([out_file]),
    )]

gotemplate = rule(
    doc = "Allow to use Go text/template package",
    implementation = _gotemplate_impl,
    attrs = {
        "template": attr.label(
            doc = "Template source",
            allow_single_file = True,
            mandatory = True,
        ),
        "_gotemplate": attr.label(
            default = "//bazel/rules/fields_go:gotemplate",
            cfg = "host",
            executable = True,
        ),
    },
)
