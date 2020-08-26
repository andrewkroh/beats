"""
Defines go_embed_fields, a custom rule for generating fields.go
files from fields.yml.
"""

def _go_embed_fields_impl(ctx):
    if ctx.attr.type == "dataset" and not ctx.attr.module:
        fail("module is required when type=dataset")

    out_file = ctx.actions.declare_file(ctx.attr.output)

    args = ctx.actions.args()
    args.add_all([
        "-i",
        ctx.file.src,
        "-beat",
        ctx.attr.beat.lower(),
        "-name",
        ctx.attr.asset_name,
        "-type",
        ctx.attr.type,
        "-pkg",
        ctx.attr.package,
        "-template",
        ctx.file._go_embed_fields_template,
        "-o",
        out_file,
        "-v",
    ])
    if ctx.attr.module:
        args.add_all(["-module", ctx.attr.module])

    ctx.actions.run(
        outputs = [out_file],
        inputs = [ctx.file.src, ctx.file._go_embed_fields_template],
        executable = ctx.executable._go_embed_fields,
        arguments = [args],
        mnemonic = "GoEmbedFields",
        progress_message = "Embedding %s into %s" % (ctx.file.src.short_path, out_file.short_path),
    )
    return [DefaultInfo(
        files = depset([out_file]),
    )]

go_embed_fields = rule(
    doc = "Create .go files that embed fields.yml data.",
    implementation = _go_embed_fields_impl,
    attrs = {
        "src": attr.label(
            doc = "fields.yml source",
            allow_single_file = True,
            mandatory = True,
        ),
        "asset_name": attr.string(
            doc = "Asset name to use in generated code",
            mandatory = True,
        ),
        "beat": attr.string(
            doc = "Name of the Beat",
            mandatory = True,
        ),
        "type": attr.string(
            doc = "Type of the fields (relates to asset.Priority)",
            values = ["ecs", "libbeat", "beat", "module", "dataset"],
            mandatory = True,
        ),
        "package": attr.string(
            doc = "Package name for generated .go file",
            mandatory = True,
        ),
        "module": attr.string(
            doc = "Name of the parent module (for type=dataset only)",
        ),
        "output": attr.string(
            doc = "Name of output file",
            default = "fields.go",
        ),
        "_go_embed_fields_template": attr.label(
            default = Label("//bazel/rules/fields:fields.go.tmpl"),
            allow_single_file = True,
        ),
        "_go_embed_fields": attr.label(
            default = "//bazel/rules/fields:go_embed_fields",
            cfg = "host",
            executable = True,
        ),
    },
)
