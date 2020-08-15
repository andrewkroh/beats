### third_party patches

These are patches for third party libraries to modify the auto-generated
BUILD.bazel files to correct issues in projects that may not follow the general
Go conventions.

After adding or changing a patch you should sanitize the patch dates to
minimize the size the diff. Use this command:

`go run sanitize_patch_dates.go *.patch`

#### Patches

##### `com_github_godror_godror.patch`

[github.com/godror/godror](https://github.com/godror/godror) provides Go
bindings for the Oracle Database Programming Interface for C. It embeds the
ODPI C files so `cc_library` needs to be created and added as a `cdep` to the
main godror `go_library` that uses cgo.

Additionally, the package uses `#include` directives for `.c` files which breaks
conventions, so we patch that behavior.
