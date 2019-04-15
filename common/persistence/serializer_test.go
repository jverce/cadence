// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package persistence

import (
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/uber-common/bark"
	workflow "github.com/uber/cadence/.gen/go/shared"
	"github.com/uber/cadence/common"
)

type (
	cadenceSerializerSuite struct {
		suite.Suite
		// override suite.Suite.Assertions with require.Assertions; this means that s.NotNil(nil) will stop the test,
		// not merely log an error
		*require.Assertions
		logger bark.Logger
	}
)

func TestCadenceSerializerSuite(t *testing.T) {
	s := new(cadenceSerializerSuite)
	suite.Run(t, s)
}

func (s *cadenceSerializerSuite) SetupTest() {
	s.logger = bark.NewLoggerFromLogrus(log.New())
	// Have to define our overridden assertions in the test setup. If we did it earlier, s.T() will return nil
	s.Assertions = require.New(s.T())
}

func (s *cadenceSerializerSuite) TestSerializer() {

	concurrency := 1
	startWG := sync.WaitGroup{}
	doneWG := sync.WaitGroup{}

	startWG.Add(1)
	doneWG.Add(concurrency)

	serializer := NewCadenceSerializer()

	event0 := &workflow.HistoryEvent{
		EventId:   common.Int64Ptr(999),
		Timestamp: common.Int64Ptr(time.Now().UnixNano()),
		EventType: common.EventTypePtr(workflow.EventTypeActivityTaskCompleted),
		ActivityTaskCompletedEventAttributes: &workflow.ActivityTaskCompletedEventAttributes{
			Result:           []byte("result-1-event-1"),
			ScheduledEventId: common.Int64Ptr(4),
			StartedEventId:   common.Int64Ptr(5),
			Identity:         common.StringPtr("event-1"),
		},
	}

	history0 := &workflow.History{Events: []*workflow.HistoryEvent{event0, event0}}

	memoFields := map[string][]byte{
		"TestField": []byte(`Test binary`),
	}
	memo0 := &workflow.Memo{Fields: memoFields}

	for i := 0; i < concurrency; i++ {

		go func() {

			startWG.Wait()
			defer doneWG.Done()

			_, err := serializer.SerializeEvent(event0, common.EncodingTypeGob)
			s.NotNil(err)
			_, ok := err.(*UnknownEncodingTypeError)
			s.True(ok)

			dJSON, err := serializer.SerializeEvent(event0, common.EncodingTypeJSON)
			s.Nil(err)
			s.NotNil(dJSON)

			dThrift, err := serializer.SerializeEvent(event0, common.EncodingTypeThriftRW)
			s.Nil(err)
			s.NotNil(dThrift)

			dEmpty, err := serializer.SerializeEvent(event0, common.EncodingType(""))
			s.Nil(err)
			s.NotNil(dEmpty)

			_, err = serializer.SerializeBatchEvents(history0.Events, common.EncodingTypeGob)
			s.NotNil(err)
			_, ok = err.(*UnknownEncodingTypeError)
			s.True(ok)

			dsJSON, err := serializer.SerializeBatchEvents(history0.Events, common.EncodingTypeJSON)
			s.Nil(err)
			s.NotNil(dsJSON)

			dsThrift, err := serializer.SerializeBatchEvents(history0.Events, common.EncodingTypeThriftRW)
			s.Nil(err)
			s.NotNil(dsThrift)

			dsEmpty, err := serializer.SerializeBatchEvents(history0.Events, common.EncodingType(""))
			s.Nil(err)
			s.NotNil(dsEmpty)

			_, err = serializer.SerializeVisibilityMemo(memo0, common.EncodingTypeGob)
			s.NotNil(err)
			_, ok = err.(*UnknownEncodingTypeError)
			s.True(ok)

			mJSON, err := serializer.SerializeVisibilityMemo(memo0, common.EncodingTypeJSON)
			s.Nil(err)
			s.NotNil(mJSON)

			mThrift, err := serializer.SerializeVisibilityMemo(memo0, common.EncodingTypeThriftRW)
			s.Nil(err)
			s.NotNil(mThrift)

			mEmpty, err := serializer.SerializeVisibilityMemo(memo0, common.EncodingType(""))
			s.Nil(err)
			s.NotNil(mEmpty)

			event1, err := serializer.DeserializeEvent(dJSON)
			s.Nil(err)
			s.True(event0.Equals(event1))

			event2, err := serializer.DeserializeEvent(dThrift)
			s.Nil(err)
			s.True(event0.Equals(event2))

			event3, err := serializer.DeserializeEvent(dEmpty)
			s.Nil(err)
			s.True(event0.Equals(event3))

			events, err := serializer.DeserializeBatchEvents(dsJSON)
			history1 := &workflow.History{Events: events}
			s.Nil(err)
			s.True(history0.Equals(history1))

			events, err = serializer.DeserializeBatchEvents(dsThrift)
			history2 := &workflow.History{Events: events}
			s.Nil(err)
			s.True(history0.Equals(history2))

			events, err = serializer.DeserializeBatchEvents(dsEmpty)
			history3 := &workflow.History{Events: events}
			s.Nil(err)
			s.True(history0.Equals(history3))

			memo1, err := serializer.DeserializeVisibilityMemo(mJSON)
			s.Nil(err)
			s.True(memo0.Equals(memo1))

			memo2, err := serializer.DeserializeVisibilityMemo(mThrift)
			s.Nil(err)
			s.True(memo0.Equals(memo2))
			memo3, err := serializer.DeserializeVisibilityMemo(mEmpty)
			s.Nil(err)
			s.True(memo0.Equals(memo3))
		}()
	}

	startWG.Done()
	succ := common.AwaitWaitGroup(&doneWG, 10*time.Second)
	s.True(succ, "test timed out")
}
