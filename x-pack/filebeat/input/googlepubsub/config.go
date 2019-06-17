package googlepubsub

type config struct {
	// Google Cloud project name.
	ProjectID string `config:"project_id" validate:"required"`

	// Google Cloud Pub/Sub topic name.
	Topic string `config:"topic" validate:"required"`

	// Google Cloud Pub/Sub subscription name. Multiple Filebeats can pull from same subscription.
	Subscription string `config:"subscription.name" validate:"required"`

	// JSON file containing authentication credentials and key.
	CredentialsFile string `config:"credentials_file" validate:"required"`
}
