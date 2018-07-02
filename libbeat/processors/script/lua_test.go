package script

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
)

func newEvent() *beat.Event {
	return &beat.Event{
		Fields: common.MapStr{
			"source": common.MapStr{
				"ip": "192.168.1.1",
			},
		},
	}
}

const putScript = `
function process(event)
  event:Put("hello", "world")
end
`

func TestLuaProcessor(t *testing.T) {
	p := &luaProcessor{Script: putScript}
	if err := p.init(); err != nil {
		t.Fatal(err)
	}

	e, err := p.Run(newEvent())
	if err != nil {
		t.Fatal(err)
	}

	v, _ := e.GetValue("hello")
	assert.Equal(t, v, "world")
}

func BenchmarkLuaProcessorRun(b *testing.B) {
	p := &luaProcessor{Script: putScript}
	if err := p.init(); err != nil {
		b.Fatal(err)
	}

	event := newEvent()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.Run(event)
		if err != nil {
			b.Fatal(err)
		}
	}
}
