package goja

import (
	"testing"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/common"
	"encoding/json"
	"github.com/elastic/beats/libbeat/beat"
	"time"
)

const sysmonNetwork = `
{
  "@timestamp": "2018-08-19T12:49:02.238Z",
  "beat": {
    "hostname": "bert",
    "name": "bert",
    "version": "6.3.2"
  },
  "computer_name": "bert",
  "event_data": {
    "DestinationHostname": "a23-67-250-128.deploy.static.akamaitechnologies.com",
    "DestinationIp": "23.67.250.128",
    "DestinationIsIpv6": "false",
    "DestinationPort": "80",
    "DestinationPortName": "http",
    "Image": "C:\\Program Files\\AVAST Software\\Avast\\AvastSvc.exe",
    "Initiated": "true",
    "ProcessGuid": "{3CE5A39C-DF41-5B72-0100-00106361E07C}",
    "ProcessId": "5456",
    "Protocol": "tcp",
    "SourceHostname": "bert.local.crowbird.com",
    "SourceIp": "10.100.2.30",
    "SourceIsIpv6": "false",
    "SourcePort": "53544",
    "User": "NT AUTHORITY\\SYSTEM",
    "UtcTime": "2018-08-19 12:48:52.220"
  },
  "event_id": 3,
  "host": {
    "architecture": "x86_64",
    "id": "3ce5a39c-1460-4be3-a015-860b121f9f06",
    "name": "bert",
    "os": {
      "build": "7601.0",
      "family": "windows",
      "platform": "windows",
      "version": "6.1"
    }
  },
  "level": "Information",
  "log_name": "Microsoft-Windows-Sysmon/Operational",
  "message": "Network connection detected:\nRuleName: \nUtcTime: 2018-08-19 12:48:52.220\nProcessGuid: {3CE5A39C-DF41-5B72-0100-00106361E07C}\nProcessId: 5456\nImage: C:\\Program Files\\AVAST Software\\Avast\\AvastSvc.exe\nUser: NT AUTHORITY\\SYSTEM\nProtocol: tcp\nInitiated: true\nSourceIsIpv6: false\nSourceIp: 10.100.2.30\nSourceHostname: bert.local.crowbird.com\nSourcePort: 53544\nSourcePortName: \nDestinationIsIpv6: false\nDestinationIp: 23.67.250.128\nDestinationHostname: a23-67-250-128.deploy.static.akamaitechnologies.com\nDestinationPort: 80\nDestinationPortName: http",
  "opcode": "Info",
  "process_id": 10328,
  "provider_guid": "{5770385F-C22A-43E0-BF4C-06F5698FFBD9}",
  "record_number": "2475",
  "source_name": "Microsoft-Windows-Sysmon",
  "task": "Network connection detected (rule: NetworkConnect)",
  "thread_id": 10168,
  "type": "wineventlog",
  "user": {
    "domain": "NT AUTHORITY",
    "identifier": "S-1-5-18",
    "name": "SYSTEM",
    "type": "User"
  },
  "version": 5
}
`

const sysmonScriptProcessor = `
function process(evt) {
    // Process.
	evt.rename('event_data.Image', 'process.exe');
    pid = evt.get('event_data.ProcessId');
    evt.delete('event_data.ProcessId');
    evt.put('process.pid', Number(pid));
	evt.rename('event_data.ProcessGuid', 'process.guid');

    // Destination.
	evt.rename('event_data.DestinationHostname', 'destination.hostname');
	evt.rename('event_data.DestinationIp',       'destination.ip');
	evt.rename('event_data.DestinationPortName', 'destination.port_name');
    port = evt.get('event_data.DestinationPort');
    evt.delete('event_data.DestinationPort');
    evt.put('destination.port', Number(port));

    // Source.
	evt.rename('event_data.SourceHostname', 'source.hostname');
	evt.rename('event_data.SourceIp', 'source.ip');
	evt.rename('event_data.SourcePortName', 'source.port_name');
    port = evt.get('event_data.SourcePort');
    evt.delete('event_data.SourcePort');
    evt.put('source.port', Number(port));

	// Network.
	evt.rename('event_data.Protocol',            'network.transport');

	// User.
    evt.delete('user');
	user = evt.get('event_data.User');
    evt.delete('event_data.User');
    userParts = user.split('\\');
	evt.put('user.name', userParts[0]);
	evt.put('user.domain', userParts[1]);

    // Timestamp.
    evt.rename('@timestamp', 'event.created');
	ts = evt.get('event_data.UtcTime');
    evt.delete('event_data.UtcTime');
    ts = ts.replace(' ', 'T');
	evt.put('@timestamp', new Date(ts));

	// Log
    evt.rename('message', 'log.message');
    evt.rename('level', 'log.level');

    // Base
    evt.rename('task', 'message');
};
`

func TestProcessSysmon(t *testing.T) {
	logp.TestingSetup()

	var m common.MapStr
	if err := json.Unmarshal([]byte(sysmonNetwork), &m); err != nil {
		t.Fatal(err)
	}

	ts, err := time.Parse(time.RFC3339, m["@timestamp"].(string))
	if err != nil {
		t.Fatal(err)
	}
	event := &beat.Event{
		Timestamp: ts,
		Fields: m,
	}

	p := &jsProcessor{Script: sysmonScriptProcessor}
	if err := p.init(); err != nil {
		t.Fatal(err)
	}

	e, err := p.Run(event)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	data, err := json.MarshalIndent(e.Fields, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(data))
}
