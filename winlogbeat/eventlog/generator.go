package eventlog

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/winlogbeat/sys"
	"strconv"
)

var (
	logonRecord = mustUnmarshal(event4624)
	logonFailureRecord = mustUnmarshal(event4625)
	validUsers = []string{"aaron", "ross", "smith", "gates", "jared", "ron", "jake",
		"tim", "john", "gordy", "kate", "kathy", "hugh", "gary", "jonah",
	    "ryan", "dan"}
	scanUsers = []string{"Administrator", "admin", "guest", "user"}
	ipAddresses = []string{
		"23.27.0.",  // U.S.
		"5.9.0.",    // Germany, Hetzner
		"14.193.0.", // Japan
		"174.13.4.",
		"114.30.7.",
		"124.11.8.",
		"134.17.9.",
		"144.19.8.",
		"200.19.8.",
		"201.19.8.",
	}
	scanIpAddresses = []string{"58.247.242."}
)

const generatorAPIName = "generator"

var generatorConfigKeys = append(commonConfigKeys, []string{"duration", "start", "interval", "event_id", "logon_type"}...)

type GeneratorConfig struct {
	ConfigCommon `config:",inline"`
	Duration time.Duration `config:"duration"` // Length of time to simulate. If not set defaults to a real-time infinite simulation.
	StartDate string       `config:"start"`    // Date/time of the first event. Defaults to now-24h.
	Interval time.Duration `config:"interval"`
	EventID  int           `config:"event_id"`
	LogonType int          `config:"logon_type"`
}

type generator struct {
	name string
	done chan struct{}
	simEvents <-chan Record // Channel holding simulated events.
	startTime time.Time // Start of the simulation.
	endTime time.Time // Start of the simulation.

	logPrefix     string               // String to prefix on log messages.
	config        GeneratorConfig
}

func (g *generator) Open(recordNumber uint64) error {
	g.simEvents = g.generate(g.done, recordNumber, g.config.EventMetadata)
	return nil
}

func (g *generator) generate(done chan struct{}, recordNumber uint64, metadata common.EventMetadata) <-chan Record {
	out := make(chan Record, 100)

	go func() {
		defer close(out)
		var runningTime time.Duration

		logp.Info("event_id: %d", g.config.EventID)

		var event sys.Event
		var users []string
		var ips []string
		switch g.config.EventID {
		case 4624:
			event = logonRecord
			users = validUsers
			ips   = ipAddresses
		case 4625:
			event = logonFailureRecord
			users = scanUsers
			ips = scanIpAddresses
		default:
			event = logonRecord
			users = validUsers
			ips   = ipAddresses
		}

		logp.Info("event.event_id: %d", event.EventIdentifier.ID)

		for {
			recordNumber++
			runningTime += arrivalTime(g.config.Interval)
			eventTime := g.startTime.Add(runningTime)

			r := Record{
				Event: copyEvent(event),
				EventMetadata: g.config.EventMetadata,
				API: generatorAPIName,
			}
			r.Event.RecordID = recordNumber
			r.Event.TimeCreated.SystemTime = eventTime

			id := rand.Intn(len(users))
			r.Computer = fmt.Sprintf("wrks-%.03d", id)
			for i := range r.EventData.Pairs {
				if r.EventData.Pairs[i].Key == "TargetUserName" {
					r.EventData.Pairs[i].Value = users[id]
				} else if r.EventData.Pairs[i].Key == "WorkstationName" {
					r.EventData.Pairs[i].Value = r.Computer
				} else if r.EventData.Pairs[i].Key == "IpAddress" {
					ip := ips[id % len(ips)]
					r.EventData.Pairs[i].Value = fmt.Sprintf("%s%d", ip, id+1)
				} else if r.EventData.Pairs[i].Key == "LogonType" {
					r.EventData.Pairs[i].Value = strconv.Itoa(g.config.LogonType)
				}
			}

			select {
			case <- done:
				return
			case out <- r:
			}

			if eventTime.After(g.endTime) {
				return
			}
		}
	}()

	return out
}

// Read records from the event log.
func (g *generator) Read() ([]Record, error) {
	logp.Info("entering Read")
	defer logp.Info("exiting Read")

	var records []Record
	loop:
	for {
		select {
		case <-g.done:
			break loop
		case r, ok := <-g.simEvents:
			if !ok {
				break loop
			}
			records = append(records, r)

			if len(records) >= 100 {
				break loop
			}
		default:
			break loop
		}
	}

	return records, nil
}

// Close the event log. It should not be re-opened after closing.
func (g *generator) Close() error {
	close(g.done)
	return nil
}

// Name returns the event log's name.
func (g *generator) Name() string {
	return g.name
}

func newGeneratorEventLog(options map[string]interface{}) (EventLog, error) {
	c := GeneratorConfig{
		StartDate: "-24h",
		Duration:  time.Duration(1<<63 - 1), // Max Duration
		Interval:  time.Minute,
		EventID:   4624,
		LogonType: 10,
	}
	if err := readConfig(options, &c, generatorConfigKeys); err != nil {
		return nil, err
	}

	var startTime time.Time
	if d, err := time.ParseDuration(c.StartDate); err == nil {
		logp.Info("ParseDuration=%v", d)
		startTime = time.Now().Add(d)
	} else {
		return nil, fmt.Errorf("failed to parse start_date %v", c.StartDate)
	}

	logp.Info("creating log %s", c.Name)
	endTime := startTime.Add(c.Duration)
	return &generator{
		name: c.Name,
		config: c,
		done: make(chan struct{}),
		startTime: startTime,
		endTime: endTime,
		simEvents: make(chan Record, 100),
	}, nil
}

func init() {
	Register(generatorAPIName, -1, newGeneratorEventLog, nil)
}

func arrivalTime(meanArrivalTime time.Duration) time.Duration {
	// Exponential distribution sample with mean rate.
	sample := rand.ExpFloat64() * float64(meanArrivalTime.Nanoseconds())
	// This truncates instead of rounding which my skew the mean.
	return time.Duration(sample)
}

func arrivalTimes(n int, meanArrivalTime time.Duration) []time.Duration {
	times := make([]time.Duration, n)
	for i := 0; i < n; i++ {
		times[i] = arrivalTime(meanArrivalTime)
	}
	return times
}

func copyEvent(event sys.Event) sys.Event {
	pairs := make([]sys.KeyValue, len(event.EventData.Pairs))
	copy(pairs, event.EventData.Pairs)
	event.EventData.Pairs = pairs

	pairs = make([]sys.KeyValue, len(event.UserData.Pairs))
	copy(pairs, event.UserData.Pairs)
	event.UserData.Pairs = pairs
	return event
}

func mustUnmarshal(xml string) sys.Event {
	event, err := sys.UnmarshalEventXML([]byte(xml))
	if err != nil {
		panic(err)
	}
	return event
}

const event4624 = `
<?xml version="1.0"?>
<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
  <System>
    <Provider Name="Microsoft-Windows-Security-Auditing" Guid="{54849625-5478-4994-A5BA-3E3B0328C30D}"/>
    <EventID>4624</EventID>
    <Version>1</Version>
    <Level>0</Level>
    <Task>12544</Task>
    <Opcode>0</Opcode>
    <Keywords>0x8020000000000000</Keywords>
    <TimeCreated SystemTime="2016-05-22T05:14:58.773337200Z"/>
    <EventRecordID>18910</EventRecordID>
    <Correlation/>
    <Execution ProcessID="664" ThreadID="1884"/>
    <Channel>Security</Channel>
    <Computer>wrks-001.elastic.co</Computer>
    <Security/>
  </System>
  <EventData>
    <Data Name="SubjectUserSid">S-1-5-18</Data>
    <Data Name="SubjectUserName">WRKS-001$</Data>
    <Data Name="SubjectDomainName">ELASTIC</Data>
    <Data Name="SubjectLogonId">0x3e7</Data>
    <Data Name="TargetUserSid">S-1-5-21-3171274759-745574661-2147429726-1116</Data>
    <Data Name="TargetUserName">ak</Data>
    <Data Name="TargetDomainName">ELASTIC</Data>
    <Data Name="TargetLogonId">0x390104</Data>
    <Data Name="LogonType">10</Data>
    <Data Name="LogonProcessName">User32 </Data>
    <Data Name="AuthenticationPackageName">Negotiate</Data>
    <Data Name="WorkstationName">WRKS-001</Data>
    <Data Name="LogonGuid">{00000000-0000-0000-0000-000000000000}</Data>
    <Data Name="TransmittedServices">-</Data>
    <Data Name="LmPackageName">-</Data>
    <Data Name="KeyLength">0</Data>
    <Data Name="ProcessId">0xf40</Data>
    <Data Name="ProcessName">C:\Windows\System32\winlogon.exe</Data>
    <Data Name="IpAddress">96.255.135.185</Data>
    <Data Name="IpPort">0</Data>
    <Data Name="ImpersonationLevel">%%1833</Data>
  </EventData>
  <RenderingInfo Culture="en-US">
    <Message>An account was successfully logged on.

Subject:
	Security ID:		S-1-5-18
	Account Name:		WRKS-001$
	Account Domain:		ELASTIC
	Logon ID:		0x3E7

Logon Type:			10

Impersonation Level:		Impersonation

New Logon:
	Security ID:		S-1-5-21-3171274759-745574661-2147429726-1116
	Account Name:		ak
	Account Domain:		ELASTIC
	Logon ID:		0x390104
	Logon GUID:		{00000000-0000-0000-0000-000000000000}

Process Information:
	Process ID:		0xf40
	Process Name:		C:\Windows\System32\winlogon.exe

Network Information:
	Workstation Name:	WRKS-001
	Source Network Address:	96.255.135.185
	Source Port:		0

Detailed Authentication Information:
	Logon Process:		User32
	Authentication Package:	Negotiate
	Transited Services:	-
	Package Name (NTLM only):	-
	Key Length:		0

This event is generated when a logon session is created. It is generated on the computer that was accessed.

The subject fields indicate the account on the local system which requested the logon. This is most commonly a service such as the Server service, or a local process such as Winlogon.exe or Services.exe.

The logon type field indicates the kind of logon that occurred. The most common types are 2 (interactive) and 3 (network).

The New Logon fields indicate the account for whom the new logon was created, i.e. the account that was logged on.

The network fields indicate where a remote logon request originated. Workstation name is not always available and may be left blank in some cases.

The impersonation level field indicates the extent to which a process in the logon session can impersonate.

The authentication information fields provide detailed information about this specific logon request.
	- Logon GUID is a unique identifier that can be used to correlate this event with a KDC event.
	- Transited services indicate which intermediate services have participated in this logon request.
	- Package name indicates which sub-protocol was used among the NTLM protocols.
	- Key length indicates the length of the generated session key. This will be 0 if no session key was requested.</Message>
    <Level>Information</Level>
    <Task>Logon</Task>
    <Opcode>Info</Opcode>
    <Channel>Security</Channel>
    <Provider>Microsoft Windows security auditing.</Provider>
    <Keywords>
      <Keyword>Audit Success</Keyword>
    </Keywords>
  </RenderingInfo>
</Event>`

const event4625 = `
<?xml version="1.0"?>
<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
  <System>
    <Provider Name="Microsoft-Windows-Security-Auditing" Guid="{54849625-5478-4994-A5BA-3E3B0328C30D}"/>
    <EventID>4625</EventID>
    <Version>0</Version>
    <Level>0</Level>
    <Task>12544</Task>
    <Opcode>0</Opcode>
    <Keywords>0x8010000000000000</Keywords>
    <TimeCreated SystemTime="2016-05-21T05:14:58.773337200Z"/>
    <EventRecordID>19397</EventRecordID>
    <Correlation/>
    <Execution ProcessID="664" ThreadID="3480"/>
    <Channel>Security</Channel>
    <Computer>wrks-001.elastic.co</Computer>
    <Security/>
  </System>
  <EventData>
    <Data Name="SubjectUserSid">S-1-5-18</Data>
    <Data Name="SubjectUserName">WRKS-001$</Data>
    <Data Name="SubjectDomainName">ELASTIC</Data>
    <Data Name="SubjectLogonId">0x3e7</Data>
    <Data Name="TargetUserSid">S-1-0-0</Data>
    <Data Name="TargetUserName">Andrew Kroh</Data>
    <Data Name="TargetDomainName">WRKS-001</Data>
    <Data Name="Status">0xc000006d</Data>
    <Data Name="FailureReason">%%2313</Data>
    <Data Name="SubStatus">0xc0000064</Data>
    <Data Name="LogonType">10</Data>
    <Data Name="LogonProcessName">User32 </Data>
    <Data Name="AuthenticationPackageName">Negotiate</Data>
    <Data Name="WorkstationName">WRKS-001</Data>
    <Data Name="TransmittedServices">-</Data>
    <Data Name="LmPackageName">-</Data>
    <Data Name="KeyLength">0</Data>
    <Data Name="ProcessId">0xf88</Data>
    <Data Name="ProcessName">C:\Windows\System32\winlogon.exe</Data>
    <Data Name="IpAddress">193.189.117.12</Data>
    <Data Name="IpPort">0</Data>
  </EventData>
  <RenderingInfo Culture="en-US">
    <Message>An account failed to log on.

Subject:
	Security ID:		S-1-5-18
	Account Name:		WRKS-001$
	Account Domain:		ELASTIC
	Logon ID:		0x3E7

Logon Type:			10

Account For Which Logon Failed:
	Security ID:		S-1-0-0
	Account Name:		Andrew Kroh
	Account Domain:		WRKS-001

Failure Information:
	Failure Reason:		Unknown user name or bad password.
	Status:			0xC000006D
	Sub Status:		0xC0000064

Process Information:
	Caller Process ID:	0xf88
	Caller Process Name:	C:\Windows\System32\winlogon.exe

Network Information:
	Workstation Name:	WRKS-001
	Source Network Address:	193.189.117.12
	Source Port:		0

Detailed Authentication Information:
	Logon Process:		User32
	Authentication Package:	Negotiate
	Transited Services:	-
	Package Name (NTLM only):	-
	Key Length:		0

This event is generated when a logon request fails. It is generated on the computer where access was attempted.

The Subject fields indicate the account on the local system which requested the logon. This is most commonly a service such as the Server service, or a local process such as Winlogon.exe or Services.exe.

The Logon Type field indicates the kind of logon that was requested. The most common types are 2 (interactive) and 3 (network).

The Process Information fields indicate which account and process on the system requested the logon.

The Network Information fields indicate where a remote logon request originated. Workstation name is not always available and may be left blank in some cases.

The authentication information fields provide detailed information about this specific logon request.
	- Transited services indicate which intermediate services have participated in this logon request.
	- Package name indicates which sub-protocol was used among the NTLM protocols.
	- Key length indicates the length of the generated session key. This will be 0 if no session key was requested.</Message>
    <Level>Information</Level>
    <Task>Logon</Task>
    <Opcode>Info</Opcode>
    <Channel>Security</Channel>
    <Provider>Microsoft Windows security auditing.</Provider>
    <Keywords>
      <Keyword>Audit Failure</Keyword>
    </Keywords>
  </RenderingInfo>
</Event>
`
