package script

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/processors"
)

func init() {
	processors.RegisterPlugin("script", newScriptProcessor)
}

func newScriptProcessor(c *common.Config) (processors.Processor, error) {
	var config = struct {
		Type string `config:"type" validate:"required"`
	}{}
	if err := c.Unpack(&config); err != nil {
		return nil, err
	}

	switch strings.ToLower(config.Type) {
	case "lua":
		return newLuaProcessorFromConfig(c)
	default:
		return nil, errors.Errorf("script type must be defined (e.g. type: lua)")
	}
}
