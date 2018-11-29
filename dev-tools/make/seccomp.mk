# Utilities for profiling binaries and generating seccomp filters based on the
# syscalls used.

#
# Variables
#
SECCOMP_BINARY    ?= ${BEAT_NAME}
SECCOMP_BLACKLIST ?= ${ES_BEATS}/libbeat/common/seccomp/seccomp-profiler-blacklist.txt
SECCOMP_ALLOWLIST ?= ${ES_BEATS}/libbeat/common/seccomp/seccomp-profiler-allow.txt

# Generates a seccomp whitelist policy for the binary pointed to by SECCOMP_BINARY.
.PHONY: seccomp
seccomp:
	@go get github.com/elastic/beats/vendor/github.com/elastic/go-seccomp-bpf/cmd/seccomp-profiler
	@test -f ${SECCOMP_BINARY} || (echo "${SECCOMP_BINARY} binary is not built."; false)
	seccomp-profiler \
	-b "$(shell grep -v ^# "${SECCOMP_BLACKLIST}")" \
	-allow "$(shell grep -v ^# "${SECCOMP_ALLOWLIST}")" \
	-t "${ES_BEATS}/libbeat/common/seccomp/policy.go.tpl" \
	-pkg include \
	-out "include/seccomp_linux_{{.GOARCH}}.go" \
	${SECCOMP_BINARY}

# Generates seccomp profiles based on the binaries produced by the package target.
.PHONY: seccomp-package
seccomp-package:
	SECCOMP_BINARY=build/golang-crossbuild/${BEAT_NAME}-linux-386 $(MAKE) seccomp
	SECCOMP_BINARY=build/golang-crossbuild/${BEAT_NAME}-linux-amd64 $(MAKE) seccomp
