package transform

import (
	"net/http"
	"io/ioutil"
	"fmt"
	"github.com/argoproj/argo-events/common"
	suuid "github.com/satori/go.uuid"
	"time"
)

// Transform request transforms http request payload into CloudEvent
func (eoc *eOperationCtx) TransformRequest(source string, r *http.Request) (*Event, error) {
	// Generate event id
	eventId := suuid.Must(suuid.NewV4())
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Errorf("failed to parse request payload. Err %+v", err)
		return nil, err
	}

	// Create an CloudEvent
	ce := &Event{
		ctx: EventContext{
			CloudEventsVersion: common.CloudEventsVersion,
			EventID:            fmt.Sprintf("%x", eventId),
			ContentType:        r.Header.Get(common.HeaderContentType),
			EventTime:          time.Now(),
			EventType:          eoc.Config.EventType,
			EventTypeVersion:   eoc.Config.EventTypeVersion,
			Source:             source,
		},
		payload: payload,
	}

	return ce, nil
}
