package wineventlog

import "testing"

func TestPublisherMetadata(t *testing.T) {
	md, err := NewPublisherMetadata(NilHandle, "Microsoft-Windows-PowerShell")
	if err != nil {
		t.Fatalf("%+v", err)
	}

	t.Run("publisher_guid", func(t *testing.T) {
		v, err := md.PublisherGUID()
		if err != nil {
			t.Fatalf("%+v", err)
		}
		t.Logf("PublisherGUID: %v", v)
	})

	t.Run("resource_file_path", func(t *testing.T) {
		v, err := md.ResourceFilePath()
		if err != nil {
			t.Fatalf("%+v", err)
		}
		t.Logf("ResourceFilePath: %v", v)
	})

	t.Run("parameter_file_path", func(t *testing.T) {
		v, err := md.ParameterFilePath()
		if err != nil {
			t.Fatalf("%+v", err)
		}
		t.Logf("ParameterFilePath: %v", v)
	})

	t.Run("message_file_path", func(t *testing.T) {
		v, err := md.MessageFilePath()
		if err != nil {
			t.Fatalf("%+v", err)
		}
		t.Logf("MessageFilePath: %v", v)
	})

	t.Run("help_link", func(t *testing.T) {
		v, err := md.HelpLink()
		if err != nil {
			t.Fatalf("%+v", err)
		}
		t.Logf("HelpLink: %v", v)
	})

	t.Run("publisher_message_id", func(t *testing.T) {
		v, err := md.PublisherMessageID()
		if err != nil {
			t.Fatalf("%+v", err)
		}
		t.Logf("PublisherMessageID: %v", v)
	})

	t.Run("keywords", func(t *testing.T) {
		values, err := md.Keywords()
		if err != nil {
			t.Fatalf("%+v", err)
		}

		if testing.Verbose() {
			for _, value := range values {
				t.Logf("%+v", value)
			}
		}
	})

	t.Run("opcodes", func(t *testing.T) {
		values, err := md.Opcodes()
		if err != nil {
			t.Fatalf("%+v", err)
		}

		if testing.Verbose() {
			for _, value := range values {
				t.Logf("%+v", value)
			}
		}
	})

	t.Run("levels", func(t *testing.T) {
		values, err := md.Levels()
		if err != nil {
			t.Fatalf("%+v", err)
		}

		if testing.Verbose() {
			for _, value := range values {
				t.Logf("%+v", value)
			}
		}
	})

	t.Run("tasks", func(t *testing.T) {
		values, err := md.Tasks()
		if err != nil {
			t.Fatalf("%+v", err)
		}

		if testing.Verbose() {
			for _, value := range values {
				t.Logf("%+v", value)
			}
		}
	})

	t.Run("channels", func(t *testing.T) {
		values, err := md.Channels()
		if err != nil {
			t.Fatalf("%+v", err)
		}

		if testing.Verbose() {
			for _, value := range values {
				t.Logf("%+v", value)
			}
		}
	})
}
