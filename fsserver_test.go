package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func NewCallbackStore() *CallbackStore {
	return &CallbackStore{
		storage: make(map[string][]string, 0),
		lock:    &sync.RWMutex{},
	}
}

func TestCallbackStoreSet(t *testing.T) {
	cbs := NewCallbackStore()
	cbs.Set("/foo", "barf")
	if cbs.storage["/foo"][0] != "barf" {
		t.Fail()
	}
}

func TestCallbackStoreGet(t *testing.T) {
	cbs := NewCallbackStore()
	cbs.Set("/foo", "barf")
	results := cbs.Get("/foo")
	if results[0] != "barf" {
		t.Errorf("Get() did not return the correct result.")
	}
	results = cbs.Get("/")
	if len(results) != 0 {
		t.Errorf("Get() did not return an empty slice.")
	}
}

func TestCallbackStoreTrigger(t *testing.T) {
	se := &SendableEvent{
		Path:  "/foo",
		Event: "CREATED",
	}
	triggered := make(chan int)
	var value []byte
	var err error
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value, err = ioutil.ReadAll(r.Body)
		if err != nil {
			t.Errorf(err.Error())
		}
		triggered <- 1
	}))
	cbs := NewCallbackStore()
	cbs.Set("/foo", ts.URL)
	cbs.Trigger("/foo", se)
	<-triggered
	var returnedEvent SendableEvent
	err = json.Unmarshal(value, &returnedEvent)
	if err != nil {
		t.Errorf(err.Error())
	}
	if returnedEvent.Path != "/foo" {
		t.Errorf("Path was not '/foo'")
	}
	if returnedEvent.Event != "CREATED" {
		t.Errorf("Event was not 'CREATED'")
	}
}

func TestNewCallbackHandler(t *testing.T) {
	cbh := NewCallbackHandler()
	if cbh == nil {
		t.Errorf("NewCallbackHandler returned nil.")
	}
	cbhType := reflect.TypeOf(cbh)
	if cbhType.String() != "*main.CallbackHandler" {
		t.Errorf("Type was %s", cbhType.String())
	}
}

func TestSetter(t *testing.T) {
	cbh := NewCallbackHandler()
	if cbh == nil {
		t.Errorf("NewCallbackHandler returned nil.")
	}
	recorder := httptest.NewRecorder()
	body := strings.NewReader("{\"Path\":\"/foo\",\"URL\":\"http://itdoesntmatter.wtf\"}")
	request, err := http.NewRequest("POST", "http://itdoesntmatter.lol", body)
	if err != nil {
		t.Errorf(err.Error())
	}
	cbh.Setter(recorder, request)
	if recorder.Code != 200 {
		t.Errorf("Status code was not 200")
	}
}

func TestGetter(t *testing.T) {
	cbh := NewCallbackHandler()
	if cbh == nil {
		t.Errorf("NewCallbackHandler returned nil.")
	}
	cbh.Set("/foo", "wut")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "http://itdoesntmatter.lol/foo", nil)
	if err != nil {
		t.Errorf(err.Error())
	}
	cbh.Getter(recorder, request)
	if recorder.Code != 200 {
		t.Errorf("Status code was not 200")
	}
	if recorder.Body == nil {
		t.Errorf("Body was nil")
	}
	if recorder.Body.String() != "[\"wut\"]" {
		t.Errorf("The body wasn't correct")
	}

}

func TestServeHTTP1(t *testing.T) {
	cbh := NewCallbackHandler()
	if cbh == nil {
		t.Errorf("NewCallbackHandler returned nil.")
	}
	cbh.Set("/foo", "wut")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "http://itdoesntmatter.lol/foo", nil)
	if err != nil {
		t.Errorf(err.Error())
	}
	cbh.ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Errorf("Status code was not 200")
	}
	if recorder.Body == nil {
		t.Errorf("Body was nil")
	}
	if recorder.Body.String() != "[\"wut\"]" {
		t.Errorf("The body wasn't correct")
	}
}

func TestServeHTTP2(t *testing.T) {
	cbh := NewCallbackHandler()
	if cbh == nil {
		t.Errorf("NewCallbackHandler returned nil.")
	}
	recorder := httptest.NewRecorder()
	body := strings.NewReader("{\"Path\":\"/foo\",\"URL\":\"http://itdoesntmatter.lol\"}")
	request, err := http.NewRequest("POST", "http://itdoesntmatter.lol", body)
	if err != nil {
		t.Errorf(err.Error())
	}
	cbh.ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Errorf("Status code was not 200")
	}
}

func TestServeHTTP3(t *testing.T) {
	cbh := NewCallbackHandler()
	if cbh == nil {
		t.Errorf("NewCallbackHandler returned nil.")
	}
	recorder := httptest.NewRecorder()
	body := strings.NewReader("{\"Path\":\"/foo\",\"URL\":\"http://itdoesntmatter.lol\"}")
	request, err := http.NewRequest("PATCH", "http://itdoesntmatter.lol", body)
	if err != nil {
		t.Errorf(err.Error())
	}
	cbh.ServeHTTP(recorder, request)
	if recorder.Code != 404 {
		t.Errorf("Status code was not 404")
	}
}

func TestPathExists(t *testing.T) {
	path, err := os.Getwd()
	if err != nil {
		t.Errorf(err.Error())
	}
	exists1, err := PathExists(path)
	if err != nil {
		t.Errorf(err.Error())
	}
	if !exists1 {
		t.Errorf("os.Getwd() doesn't exist? Really?")
	}
	exists2, err := PathExists("/asdfasdf")
	if err != nil {
		t.Errorf(err.Error())
	}
	if exists2 {
		t.Errorf("Uh, that shouldn't exist. Check /asdfasdf")
	}

}
