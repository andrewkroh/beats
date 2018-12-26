#
# Variables
#
ES_BEATS := ..
GOX_OS   := windows

#
# Includes
#
include $(ES_BEATS)/dev-tools/make/oss.mk

#
# Targets
#

.PHONY: gen
gen:
	GOOS=windows GOARCH=386 go generate -v -x ./...
