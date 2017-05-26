package main

import (
	"net/http"
	"time"

	"github.com/newrelic/go-agent"
)

type NewRelicStubApplication struct{}

func (a NewRelicStubApplication) StartTransaction(name string, w http.ResponseWriter, r *http.Request) newrelic.Transaction {
	return &NewRelicStubTransaction{w}
}

func (a NewRelicStubApplication) RecordCustomEvent(eventType string, params map[string]interface{}) error {
	return nil
}

func (a NewRelicStubApplication) WaitForConnection(timeout time.Duration) error {
	return nil
}

func (a NewRelicStubApplication) Shutdown(timeout time.Duration) {
}

type NewRelicStubTransaction struct {
	http.ResponseWriter
}

func (t *NewRelicStubTransaction) End() error {
	return nil
}

func (t *NewRelicStubTransaction) Ignore() error {
	return nil
}

func (t *NewRelicStubTransaction) SetName(name string) error {
	return nil
}

func (t *NewRelicStubTransaction) NoticeError(err error) error {
	return nil
}

func (t *NewRelicStubTransaction) AddAttribute(key string, value interface{}) error {
	return nil
}

func (t *NewRelicStubTransaction) StartSegmentNow() newrelic.SegmentStartTime {
	return newrelic.SegmentStartTime{}
}
