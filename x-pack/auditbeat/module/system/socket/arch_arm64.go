// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

//go:build linux
// +build linux

package socket

import "github.com/elastic/elastic-agent-libs/mapstr"

var archVariables = mapstr.M{
	// Regular function call parameters 1 to 6
	"P1": "%x0",
	"P2": "%x1",
	"P3": "%x2",
	"P4": "%x3",
	"P5": "%x4",
	"P6": "%x5",

	// System call parameters. These are temporary, the definitive SYS_Px args
	// will be determined by guess/syscallargs.go.
	"_SYS_P1": "%x0",
	"_SYS_P2": "%x1",
	"_SYS_P3": "%x2",
	"_SYS_P4": "%x3",
	"_SYS_P5": "%x4",
	"_SYS_P6": "%x5",

	"RET": "%x0",
}
