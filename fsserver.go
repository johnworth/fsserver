package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"code.google.com/p/go.exp/fsnotify"
)

// SendableEvent encapsulates the data sent to callback URLs when a watched file
// changes.
type SendableEvent struct {
	Path  string
	Event string
}

// CallbackStorer is the interface that storage mechanisms for callbacks need to
// implement
type CallbackStorer interface {
	Set(string, string)
	Get(string) []string
	Trigger(string) error
}

// CallbackStore is an in-memory implementation of CallbackStorer.
type CallbackStore struct {
	storage map[string][]string
	lock    *sync.RWMutex
}

// Set associates a callback with a path. Neither path or callback are currently
// validated. Not validating the path allows callers to set a callback for a
// path that doesn't exist yet.
func (c *CallbackStore) Set(path string, cb string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	cbs, ok := c.storage[path]
	if !ok {
		c.storage[path] = make([]string, 0)
		cbs = c.storage[path]
	}
	cbs = append(cbs, cb)
	c.storage[path] = cbs
}

// Get returns a []string containing the callback URLs (as strings) for the
// given path.
func (c *CallbackStore) Get(path string) []string {
	c.lock.RLock()
	defer c.lock.RUnlock()
	cbs, ok := c.storage[path]
	if !ok {
		c.storage[path] = make([]string, 0)
		cbs = c.storage[path]
	}
	return cbs
}

// Trigger will cause a JSON-encoded the SendableEvent to be sent to the
// callback URLs associated with the given path. The requests are POSTs and they
// are performed asynchronously.
func (c *CallbackStore) Trigger(path string, se *SendableEvent) error {
	cbs := c.Get(path)
	msg, err := json.Marshal(se)
	if err != nil {
		return err
	}
	go func() {
		for _, cb := range cbs {
			resp, err := http.Post(cb, "application/json", bytes.NewBuffer(msg))
			if err != nil {
				log.Printf(err.Error())
			} else {
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Printf(err.Error())
				}
				log.Printf(
					"Path: %s\nURL:%s\nStatus: %d\nBody:\n%s\n",
					path,
					cb,
					resp.StatusCode,
					string(body[:]),
				)
			}
		}
	}()
	return err
}

// CallbackHandler implements the HTTP request handling for the embedded
// CallbackStore. That means you can call all of the CallbackStore methods on
// an instance of CallbackHandler.
type CallbackHandler struct {
	CallbackStore
}

// NewCallbackHandler returns a pointer to a new instance of CallbackHandler.
func NewCallbackHandler() *CallbackHandler {
	ch := &CallbackHandler{
		CallbackStore{
			storage: make(map[string][]string, 0),
			lock:    &sync.RWMutex{},
		},
	}
	return ch
}

// Getter handles HTTP requests for getting the callbacks associated with a
// path.
func (c *CallbackHandler) Getter(resp http.ResponseWriter, request *http.Request) {
	if request.Method != "GET" {
		http.Error(resp, "Not Found!", 404)
		return
	}
	path := request.URL.Path
	cbs := c.Get(path)
	marshalled, err := json.Marshal(cbs)
	if err != nil {
		http.Error(resp, err.Error(), 500)
		return
	}
	io.Copy(resp, bytes.NewBuffer(marshalled))
}

type setCallback struct {
	Path string
	URL  string
}

// Setter handles HTTP requests for setting the callbacks associated with a
// path.
func (c *CallbackHandler) Setter(resp http.ResponseWriter, request *http.Request) {
	if request.Method != "POST" {
		http.Error(resp, "Not Found!", 404)
		return
	}
	if request.Body == nil {
		http.Error(resp, "Body was empty.", 500)
		return
	}
	var setter setCallback
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		http.Error(resp, err.Error(), 500)
		return
	}
	err = json.Unmarshal(body, &setter)
	if err != nil {
		http.Error(resp, err.Error(), 500)
		return
	}
	c.Set(setter.Path, setter.URL)
}

// Route implements the logic for delegating a request to either Setter() or
// Getter().
func (c *CallbackHandler) ServeHTTP(resp http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case "GET":
		c.Getter(resp, request)
	case "POST":
		c.Setter(resp, request)
	default:
		http.Error(resp, "Not Found!", 404)
		return
	}
}

// PathExists returns true if a path exists on the filesystem and false
// otherwise. If an error occurs (other than a NotExist) then the error will
// be returned along with false.
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, nil
}

// StringifyEvent translates a *fsnotify.FileEvent into a string. Useful for
// SendableEvents and logging.
func StringifyEvent(event *fsnotify.FileEvent) string {
	if event.IsAttrib() {
		return "Attrib"
	}
	if event.IsCreate() {
		return "Create"
	}
	if event.IsDelete() {
		return "Delete"
	}
	if event.IsModify() {
		return "Modify"
	}
	if event.IsRename() {
		return "Rename"
	}
	return "Unknown"
}

// Monitor creates a fsnotify.Watcher for the given path and sends
// SendableEvents out on the provided out channel.
func Monitor(path string, out chan<- SendableEvent) error {
	exists, err := PathExists(path)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%s does not exist", path)
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	go func() {
		for {
			select {
			case event := <-watcher.Event:
				sendable := SendableEvent{
					Path:  event.Name,
					Event: StringifyEvent(event),
				}
				out <- sendable
			case err := <-watcher.Error:
				log.Println(err)
			}
		}
	}()
	err = watcher.Watch(path)
	return err
}

func main() {
	var path string
	var port string
	flag.StringVar(&path, "path", "", "The path to watch")
	flag.StringVar(&port, "port", "8080", "The port to listen on.")
	flag.Parse()
	if path == "" {
		log.Fatal("Path was not set.")
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}
	handler := NewCallbackHandler()
	in := make(chan SendableEvent)
	go func() {
		for {
			select {
			case se := <-in:
				log.Printf("%s\t%s\n", se.Event, se.Path)
				handler.Trigger(se.Path, &se)
			}
		}
	}()
	err := Monitor(path, in)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		http.Handle(
			"/callbacks/",
			http.StripPrefix("/callbacks/", handler),
		)
		http.Handle(
			"/files/",
			http.StripPrefix("/files/", http.FileServer(http.Dir(path))),
		)
		log.Fatal(http.ListenAndServe(port, nil))
	}()
	block := make(chan int)
	<-block
}
