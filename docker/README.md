# Docker Images

This package specifies targets for loading Docker images into the Docker daemon.
Images are declared via [workspace rules](
https://docs.bazel.build/versions/master/be/workspace.html) similar to how Go
dependencies are declared. These targets can be depended on from integration
tests that need access to images.

The state of the Docker daemon **is not tracked** by Bazel. Rules that load
images do not cache and will always be executed to ensure images are loaded.

For debugging, this command will show the output files created by these targets:

```
bazel aquery '//docker:*' | grep Outputs | sed 's/.*\[\(.*\)\]/\1/g'
```

