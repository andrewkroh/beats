// +build mage

package main

import (
	"github.com/elastic/beats/dev-tools/mage"

	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/common"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/build"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/pkg"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/dashboard"
	// mage:import
	_ "github.com/elastic/beats/dev-tools/mage/target/unittest"
	// mage:import
	heartbeat "github.com/elastic/beats/heartbeat/scripts/mage"
)

func init() {
	heartbeat.SelectLogic = mage.XPackProject

	mage.BeatDescription = "Ping remote services for availability and log " +
		"results to Elasticsearch or send to Logstash."
	mage.BeatServiceName = "heartbeat-elastic"
}
