package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type DummyFetcher struct{}

func (d DummyFetcher) FetchRate(_ctx context.Context) (float64, error) {
	return 42, nil
}

func tmust(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

type testState struct {
	t        *testing.T
	id       uuid.UUID
	listener net.Listener
	emails   *EmailDB
    alreadyClosed bool
}

func (s *testState) Close() {
    if !s.alreadyClosed {
        s.listener.Close()
        s.emails.Close()
        os.RemoveAll("test_data")
    }
    s.alreadyClosed = true
}

func (s *testState) APIURL(path string) string {
	return "http://" + s.listener.Addr().String() + path
}

func prepare(t *testing.T) testState {
	return prepareWithExistingId(t, uuid.New())
}

func (s *testState) restart() testState {
	s.listener.Close()
	s.emails.Close()
    s.alreadyClosed = true
	return prepareWithExistingId(s.t, s.id)
}

func prepareWithExistingId(t *testing.T, id uuid.UUID) testState {
	logger := zerolog.Nop()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	tmust(t, err)

	os.Mkdir("test_data", 0755)
	emails, err := NewEmailDB("test_data/" + id.String() + ".txt")
	tmust(t, err)

	router := Bootstrap(BootstrapOpts{
		Emails:  emails,
		Logger:  logger,
		Fetcher: DummyFetcher{},
	})

	go func() {
		http.Serve(listener, router)
	}()

	return testState{
		t:        t,
		id:       id,
		listener: listener,
		emails:   emails,
	}
}

func TestFetchRate(t *testing.T) {
	state := prepare(t)
	defer state.Close()

	resp, err := http.Get(state.APIURL("/rate"))
	tmust(t, err)

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}
	var res *float64
	body, err := io.ReadAll(resp.Body)
	tmust(t, err)
	err = json.Unmarshal(body, &res)
	tmust(t, err)

	if *res != 42 {
		t.Fatalf("Expected 42, got %f", *res)
	}
}

func TestSubscribe(t *testing.T) {
	state := prepare(t)
	defer state.Close()

	buffer := bytes.NewBufferString("email=foo@mail.com")
	resp, err := http.Post(state.APIURL("/subscribe"), "application/x-www-form-urlencoded", buffer)
	tmust(t, err)

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 201, got %d", resp.StatusCode)
	}

	buffer.Reset()
	buffer.WriteString("email=foo@mail.com")

    // Saving the same email twice should result in a 400
	resp, err = http.Post(state.APIURL("/subscribe"), "application/x-www-form-urlencoded", buffer)
	tmust(t, err)

	if resp.StatusCode != 400 {
		t.Fatalf("Expected 400, got %d", resp.StatusCode)
	}
}

func TestEmailIsPersisted(t *testing.T) {
	state1 := prepare(t)
	defer state1.Close()

	buffer := bytes.NewBufferString("email=foo@mail.com")
	resp, err := http.Post(state1.APIURL("/subscribe"), "application/x-www-form-urlencoded", buffer)
	tmust(t, err)

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 201, got %d", resp.StatusCode)
	}

	buffer.Reset()
	buffer.WriteString("email=foo@mail.com")

    // Restarting the server from the ground up should still have the emails persisted
	state2 := state1.restart()
    defer state2.Close()

	buffer.Reset()
	buffer.WriteString("email=foo@mail.com")
	resp, err = http.Post(state2.APIURL("/subscribe"), "application/x-www-form-urlencoded", buffer)
	tmust(t, err)

	if resp.StatusCode != 400 {
		t.Fatalf("Expected 400, got %d", resp.StatusCode)
	}
}
