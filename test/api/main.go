// Copyright IBM Corp. 2020, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
)

func main() {
	// Extension API endpoints.
	http.HandleFunc("/2020-01-01/extension/register", extensionRegisterHandler)
	http.HandleFunc("/2020-01-01/extension/event/next", extensionEventHandler())

	// Test framework synchronisation endpoints.
	// GETs will wait for a POST to the path.
	// POSTs return immediately.
	http.HandleFunc("/_sync/extension-initialised", syncHandler())
	http.HandleFunc("/_sync/shutdown-extension", syncHandler())

	log.Println("Mock API listening on port 80")
	err := http.ListenAndServe("0.0.0.0:80", nil)
	if err != nil {
		log.Printf("Error shutting down: %s\n", err)
	}
}

// extensionRegisterHandler just answers OK.
func extensionRegisterHandler(w http.ResponseWriter, _ *http.Request) {
	log.Println("/extension/register API invoked")
	_, err := w.Write([]byte("{}"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
}

// extensionEventHandler returns an INVOKE event and then a SHUTDOWN event, with a wait in between.
func extensionEventHandler() func(w http.ResponseWriter, _ *http.Request) {
	var mutex sync.Mutex
	var calls int
	return func(w http.ResponseWriter, _ *http.Request) {
		// Make this stateful handler explicitly single threaded.
		mutex.Lock()
		defer mutex.Unlock()

		log.Println("/extension/event/next API invoked")
		calls++
		switch calls {
		case 1:
			// Tell the API that the extension has finished initialising.
			_, err := http.Post("http://127.0.0.1:80/_sync/extension-initialised", "", nil)
			if err != nil {
				http.Error(w, "Failed to create extension initialised event", 500)
				return
			}
			_, err = w.Write([]byte(`{"eventType":"INVOKE"}`))
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			log.Println("INVOKE event returned")
		case 2:
			// First, wait for shutdown to get requested.
			_, err := http.Get("http://127.0.0.1:80/_sync/shutdown-extension")
			if err != nil {
				http.Error(w, "Failed to wait for shutdown event", 500)
				return
			}
			_, err = w.Write([]byte(`{"eventType":"SHUTDOWN"}`))
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			log.Println("SHUTDOWN event returned")
		default:
			http.Error(w, "only expected /event/next endpoint to be called twice", 400)
		}
	}
}

func syncHandler() func(http.ResponseWriter, *http.Request) {
	ch := make(chan struct{})
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s to %s\n", r.Method, r.URL.Path)
		switch r.Method {
		case "POST":
			ch <- struct{}{}
		case "GET":
			<-ch
		default:
			http.Error(w, fmt.Sprintf("Unexpected sync method %s", r.Method), 400)
		}
	}
}
