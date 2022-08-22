// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package macunifiedlog

import (
	"fmt"
	"time"
)

type config struct {
	ID          string        `config:"id" validate:"required"` // Unique ID for the input. Used to persist state.
	IgnoreOlder time.Duration `config:"ignore_older"`           // Ignore logs older than the specified duration. If a cursor exists then ignore_older is ignored.
	Predicates  []string      `config:"predicates"`             // List of predicates for filtering logs. Multiple predicates are joined with an OR operator.
}

func (c *config) Validate() error {
	if c.IgnoreOlder < 0 {
		return fmt.Errorf("invalid ignore_older %q: value cannot be negative", c.IgnoreOlder)
	}

	return nil
}
