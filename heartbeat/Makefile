BEAT_NAME=heartbeat
BEAT_TITLE=Heartbeat
SYSTEM_TESTS=true
TEST_ENVIRONMENT?=true
ES_BEATS=..

# Path to the libbeat Makefile
include ${ES_BEATS}/libbeat/scripts/Makefile

# Use the legacy update-common target until 'mage update' is implemented.
.PHONY: update
update: update-common

# Collects all dependencies and then calls update
.PHONY: collect
collect: imports kibana

# Generate imports for all monitors
.PHONY: imports
imports: mage
	mage includeList

# Collects all module dashboards
.PHONY: kibana
kibana: mage
	mage dashboards

# Create a new monitor. Requires the parameter MONITOR
.PHONY: create-monitor
create-monitor:
	go run scripts/generate_monitor/main.go -monitor=${MONITOR}
