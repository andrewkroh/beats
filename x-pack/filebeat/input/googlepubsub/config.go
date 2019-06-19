// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package googlepubsub

type config struct {
	// Google Cloud project name.
	ProjectID string `config:"project_id" validate:"required"`

	// Google Cloud Pub/Sub topic name.
	Topic string `config:"topic" validate:"required"`

	// Google Cloud Pub/Sub subscription name. Multiple Filebeats can pull from same subscription.
	Subscription struct {
		Name                   string `config:"name" validate:"required"`
		NumGoroutines          int `config:"num_goroutines"`
		MaxOutstandingMessages int`config:"max_outstanding_messages"`
	} `config:"subscription"`

	// JSON file containing authentication credentials and key.
	CredentialsFile string `config:"credentials_file" validate:"required"`
}

func defaultConfig() config {
	var c config
	c.Subscription.NumGoroutines = 1
	c.Subscription.MaxOutstandingMessages = 1000
	return c
}
