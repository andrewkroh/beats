BUILD_DIR=$(CURDIR)/build
BEATS?=auditbeat filebeat heartbeat journalbeat metricbeat packetbeat winlogbeat x-pack/functionbeat
PROJECTS=libbeat $(BEATS)
FIND=find . -type f -not -path "*/vendor/*" -not -path "*/build/*" -not -path "*/.git/*"
.DEFAULT_GOAL := help

#
# Includes
#
include dev-tools/make/mage.mk
include dev-tools/make/reviewdog.mk

# Default target.
.PHONY: help
help:
	@echo Use mage rather than make. Here are the available mage targets:
	@mage -l

.PHONY: testsuite
testsuite:
	mage test:all

.PHONY: setup-commit-hook
setup-commit-hook:
	@cp script/pre_commit.sh .git/hooks/pre-commit
	@chmod 751 .git/hooks/pre-commit

# TODO: add gox to mage.
# Crosscompile all beats.
.PHONY: crosscompile
crosscompile:
	mage build:gox

# TODO: add coverage collection to mage.

.PHONY: update
update:
	mage update:all

.PHONY: clean
clean: mage
	mage clean

# Cleans up the vendor directory from unnecessary files
# This should always be run after updating the dependencies
.PHONY: clean-vendor
clean-vendor:
	@sh script/clean_vendor.sh

.PHONY: check
check: mage
	mage check:all

# Corrects spelling errors.
.PHONY: misspell
misspell:
	go get -u github.com/client9/misspell/cmd/misspell
	# Ignore Kibana files (.json)
	$(FIND) \
		-not -path "*.json" \
		-not -path "*.log" \
		-name '*' \
		-exec misspell -w {} \;

.PHONY: fmt
fmt: mage
	mage fmt

# Builds the documents for each beat
.PHONY: docs
docs:
	mage docs

.PHONY: notice
notice: mage
	mage update:notice

# Tests if apm works with the current code
.PHONY: test-apm
test-apm:
	sh ./script/test_apm.sh

### Packaging targets ####

# Builds a snapshot release.
.PHONY: snapshot
snapshot:
	@$(MAKE) SNAPSHOT=true release

# Builds a release.
.PHONY: release
release: mage
	mage package:all

# Builds a snapshot release. The Go version defined in .go-version will be
# installed and used for the build.
.PHONY: release-manager-snapshot
release-manager-snapshot:
	@$(MAKE) SNAPSHOT=true release-manager-release

# Builds a snapshot release. The Go version defined in .go-version will be
# installed and used for the build.
.PHONY: release-manager-release
release-manager-release:
	./dev-tools/run_with_go_ver $(MAKE) release

# Collects dashboards from all Beats and generates a zip file distribution.
.PHONY: beats-dashboards
beats-dashboards: mage
	mage package:dashboards
