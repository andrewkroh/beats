def _docker_load(ctx):
    name = ctx.attr.name
    image = ctx.file.image
    out_file = ctx.actions.declare_file("image." + ctx.label.name + ".json")

    toolchain_info = ctx.toolchains["@io_bazel_rules_docker//toolchains/docker:toolchain_type"].info

    args = ctx.actions.args()
    args.add_all([
        "-docker-tool",
        toolchain_info.tool_path,
        "-image",
        image.path,
        "-tag",
        "bazel/" + ctx.label.name + ":latest",
        "-o",
        out_file,
        # Enable verbose logging.
        #"-v",
    ])
    args.add_all(toolchain_info.docker_flags, before_each = "-docker-flag")

    ctx.actions.run(
        inputs = [image],
        outputs = [out_file],
        executable = ctx.executable._go_docker_loader,
        arguments = [args],
        use_default_shell_env = True,
        progress_message = "Loading %s docker image" % name,
        mnemonic = "GoDockerLoader",
        execution_requirements = {
            "no-cache": "1",
        },
    )

    return [DefaultInfo(files = depset([out_file]))]

docker_load = rule(
    attrs = {
        "image": attr.label(
            doc = "The image to run the commands in.",
            mandatory = True,
            allow_single_file = True,
            cfg = "target",
        ),
        "_go_docker_loader": attr.label(
            default = "//bazel/rules/docker:go_docker_loader",
            cfg = "host",
            executable = True,
        ),
    },
    doc = "This rule load a container image into Docker and tags it.",
    executable = False,
    implementation = _docker_load,
    toolchains = ["@io_bazel_rules_docker//toolchains/docker:toolchain_type"],
)
