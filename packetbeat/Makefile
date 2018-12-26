#
# Variables
#
ES_BEATS  := ..

#
# Includes
#
include $(ES_BEATS)/dev-tools/make/oss.mk

#
# Targets
#
# TODO (andrewkroh on 12-26-2018): These targets need refactored to work in the
# Mage based build system.
#

.PHONY: benchmark
benchmark:
	go test -short -bench=. ./... -cpu=2

.PHONY: create-tcp-protocol
create-tcp-protocol: python-env
	@${PYTHON_ENV}/bin/python ${ES_BEATS}/packetbeat/scripts/create_tcp_protocol.py
