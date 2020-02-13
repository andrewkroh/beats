package wineventlog

import (
	"bytes"
	"fmt"
	"strconv"
	"time"
	"unsafe"

	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/winlogbeat/sys"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

type Renderer struct {
	eventMetadata map[string]*PublisherEventMetadata
	systemContext EvtHandle // Render context for system values.
	userContext   EvtHandle // Render context for user values (event data).
	log           *logp.Logger
	buf           []byte
}

func NewRenderer() (*Renderer, error) {
	systemContext, err := _EvtCreateRenderContext(0, 0, EvtRenderContextSystem)
	if err != nil {
		return nil, err
	}
	userContext, err := _EvtCreateRenderContext(0, 0, EvtRenderContextUser)
	if err != nil {
		return nil, err
	}
	return &Renderer{
		eventMetadata: map[string]*PublisherEventMetadata{},
		systemContext: systemContext,
		userContext:   userContext,
		log:           logp.NewLogger("renderer"),
	}, nil
}

func (r *Renderer) Close() error {
	_EvtClose(r.systemContext)
	return _EvtClose(r.userContext)
}

func (r *Renderer) Render(handle EvtHandle) (*sys.Event, error) {
	var event sys.Event
	if err := r.renderSystem(handle, &event); err != nil {
		return nil, errors.Wrap(err, "failed to render system data")
	}
	if err := r.renderUser(handle, &event); err != nil {
		return nil, errors.Wrapf(err, "failed to render user/event data for provider=%v, event_id=%v",
			event.Provider.Name, event.EventIdentifier.ID)
	}
	if err := r.formatMessages(handle, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *Renderer) renderSystem(handle EvtHandle, event *sys.Event) error {
	buf, propertyCount, err := r.render(r.systemContext, handle)
	if err != nil {
		return errors.Wrap(err, "failed to get system values")
	}

	for i := 0; i < int(propertyCount); i++ {
		property := EvtSystemPropertyID(i)
		offset := i * int(sizeofEvtVariant)
		evtVar := (*EvtVariant)(unsafe.Pointer(&buf[offset]))

		data, err := evtVar.Data(buf)
		r.log.Debugf("name=%v, type=%v, is_array=%v, data=%v, data_error=%v",
			property, evtVar.Type, evtVar.Type.IsArray(), data, err)
		if err != nil || data == nil {
			continue
		}

		switch property {
		case EvtSystemProviderName:
			event.Provider.Name = data.(string)
		case EvtSystemProviderGuid:
			event.Provider.GUID = data.(windows.GUID).String()
		case EvtSystemEventID:
			event.EventIdentifier.ID = uint32(data.(uint16))
		case EvtSystemQualifiers:
			event.EventIdentifier.Qualifiers = data.(uint16)
		case EvtSystemLevel:
			event.LevelRaw = data.(uint8)
		case EvtSystemTask:
			event.TaskRaw = data.(uint16)
		case EvtSystemOpcode:
			event.OpcodeRaw = data.(uint8)
		case EvtSystemKeywords:
			event.KeywordsRaw = data.(int64)
		case EvtSystemTimeCreated:
			event.TimeCreated.SystemTime = data.(time.Time)
		case EvtSystemEventRecordId:
			event.RecordID = data.(uint64)
		case EvtSystemActivityID:
			event.Correlation.ActivityID = data.(windows.GUID).String()
		case EvtSystemRelatedActivityID:
			event.Correlation.RelatedActivityID = data.(windows.GUID).String()
		case EvtSystemProcessID:
			event.Execution.ProcessID = data.(uint32)
		case EvtSystemThreadID:
			event.Execution.ThreadID = data.(uint32)
		case EvtSystemChannel:
			event.Channel = data.(string)
		case EvtSystemComputer:
			event.Computer = data.(string)
		case EvtSystemUserID:
			event.User.Identifier = data.(string)
		case EvtSystemVersion:
			event.Version = sys.Version(data.(uint8))
		}
	}
	return nil
}

func (r *Renderer) renderUser(handle EvtHandle, event *sys.Event) error {
	meta, found := r.eventMetadata[event.Provider.Name]
	if !found {
		var err error
		meta, err = DumpEventMetadata(event.Provider.Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get event metadata for %v", event.Provider.Name)
		}
		r.eventMetadata[event.Provider.Name] = meta
	}

	buf, propertyCount, err := r.render(r.userContext, handle)
	if err != nil {
		return errors.Wrap(err, "failed to get user values")
	}
	if propertyCount == 0 {
		return nil
	}

	isClassic := event.KeywordsRaw & KeywordEventLogClassic > 0
	r.log.Debugw("Classic?", "is_classic", isClassic, "event_id", event.EventIdentifier.ID, "provider", event.Provider.Name)

	for i := 0; i < propertyCount; i++ {
		offset := i * int(sizeofEvtVariant)
		evtVar := (*EvtVariant)(unsafe.Pointer(&buf[offset]))

		var name string
		if isClassic {
			name = "param" + strconv.Itoa(i)
		} else {
			name, err = meta.LookupParam(event.EventIdentifier.ID, i)
			if err != nil {
				return errors.Wrapf(err, "failed to find name of parameter at index %v", i)
			}
		}

		data, err := evtVar.Data(buf)
		r.log.Debugf("name=%v, type=%v, is_array=%v, data=%v, err=%v", name, evtVar.Type, evtVar.Type.IsArray(), data, err)
		if err != nil {
			continue
		}

		if dataStr, ok := data.(string); ok {
			event.EventData.Pairs = append(event.EventData.Pairs, sys.KeyValue{Key: name, Value: dataStr})
		} else {
			event.EventData.Pairs = append(event.EventData.Pairs, sys.KeyValue{Key: name, Value: fmt.Sprintf("%v", data)})
		}
	}

	return nil
}

func (r *Renderer) render(context EvtHandle, eventHandle EvtHandle) ([]byte, int, error) {
	var bufferUsed, propertyCount uint32
	err := _EvtRender(context, eventHandle, EvtRenderEventValues, 0, nil, &bufferUsed, &propertyCount)
	if err != nil && err != ERROR_INSUFFICIENT_BUFFER {
		return nil, 0, errors.Errorf("expected ERROR_INSUFFICIENT_BUFFER but got %v", err)
	}
	if propertyCount == 0 {
		return nil, 0, nil
	}

	buf := r.getBuf(bufferUsed)
	err = _EvtRender(context, eventHandle, EvtRenderEventValues, bufferUsed, &buf[0], &bufferUsed, &propertyCount)
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to get values")
	}

	return buf, int(propertyCount), nil
}

func (r *Renderer) getBuf(length uint32) []byte {
	if cap(r.buf) < int(length) {
		r.buf = make([]byte, length)
	}
	r.buf = r.buf[0:length]
	return r.buf
}

func (r *Renderer) formatMessages(eventHandle EvtHandle, event *sys.Event) error {
	//providerPtr, err := syscall.UTF16PtrFromString(event.Provider.Name)
	//if err != nil {
	//	return errors.Wrap(err, "UTF16PtrFromString")
	//}
	//publisherMetadataHandle, err := _EvtOpenPublisherMetadata(0, providerPtr, nil, 0, 0)
	//if err != nil {
	//	return errors.Wrap(err, "_EvtOpenPublisherMetadata")
	//}
	//defer _EvtClose(publisherMetadataHandle)

	return r.formatMessageFromTemplate(event)
	//event.Message, _ = r.formatMessage(publisherMetadataHandle, eventHandle, EvtFormatMessageEvent)
	//event.Level, _ = r.formatMessage(publisherMetadataHandle, eventHandle, EvtFormatMessageLevel)
	//event.Task, _ = r.formatMessage(publisherMetadataHandle, eventHandle, EvtFormatMessageTask)
	//event.Channel, _ = r.formatMessage(publisherMetadataHandle, eventHandle, EvtFormatMessageChannel)
	//event.Opcode, _ = r.formatMessage(publisherMetadataHandle, eventHandle, EvtFormatMessageOpcode)
	//keyword, _ := r.formatMessage(publisherMetadataHandle, eventHandle, EvtFormatMessageKeyword)
	//if keyword !=  "" {
	//	event.Keywords = []string{keyword}
	//}
	//return err
}

func (r *Renderer) formatMessageFromTemplate(event *sys.Event) error {
	md, found := r.eventMetadata[event.Provider.Name]
	if !found {
		return errors.Errorf("failed to get metadata for %v", event.Provider.Name)
	}
	msgTmpl, found := md.Messages[event.EventIdentifier.ID]
	if !found {
		return errors.Errorf("failed to get message template for %v event_id=%d event=%#v", event.Provider.Name, event.EventIdentifier.ID, event)
	}

	kv := make(map[string]string, len(event.EventData.Pairs))
	for _, pair := range event.EventData.Pairs {
		kv[eventDataNameTransform.Replace(pair.Key)] = pair.Value
	}
	buf := bytes.NewBuffer(nil)

	err := msgTmpl.Execute(buf, kv)
	if err != nil {
		return err
	}
	event.Message = buf.String()
	return nil
}

func (r *Renderer) formatMessage(metadataHandle EvtHandle, eventHandle EvtHandle, messageFlag EvtFormatMessageFlag) (string, error) {
	var bufferUsed uint32
	err := _EvtFormatMessage(metadataHandle, eventHandle, 0, 0, 0, messageFlag, 0, nil, &bufferUsed)
	if err != ERROR_INSUFFICIENT_BUFFER {
		return "", errors.Errorf("expected ERROR_INSUFFICIENT_BUFFER but got %v", err)
	}

	buf := r.getBuf(bufferUsed * 2)
	err = _EvtFormatMessage(metadataHandle, eventHandle, 0, 0, 0, messageFlag, uint32(len(buf)/2), &buf[0], &bufferUsed)
	if err != nil {
		return "", errors.Wrapf(err, "_EvtFormatMessage for %v failed", messageFlag)
	}

	s, _, err := sys.UTF16BytesToString(buf)
	return s, err
}

const (
	KeywordAuditFailure     = 0x10000000000000
	KeywordAuditSuccess     = 0x20000000000000
	KeywordCorrelationHint  = 0x10000000000000
	KeywordCorrelationHint2 = 0x40000000000000
	KeywordEventLogClassic  = 0x80000000000000
)
// https://docs.microsoft.com/en-us/dotnet/api/system.diagnostics.eventing.reader.standardeventkeywords?view=netframework-4.8
func standardKeywords(keyword int64) ([]string, error) {
	return nil, nil
}
