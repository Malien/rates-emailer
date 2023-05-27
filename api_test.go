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

type DontSendMailer struct {
	t *testing.T
}

func (d DontSendMailer) Send(_ctx context.Context, to []string, subject string, body string) error {
	d.t.Fatalf("Unexpected call to Send")
	return nil
}

type DummyMailer struct {
	t        *testing.T
	assertFn func(to []string, subject string, body string) error
}

func (d DummyMailer) Send(_ctx context.Context, to []string, subject string, body string) error {
	return d.assertFn(to, subject, body)
}

func tmust(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

type testState struct {
	t             *testing.T
	id            uuid.UUID
	listener      net.Listener
	emails        *EmailDB
	mailer        Mailer
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

type prepareOpts struct {
	t      *testing.T
	id     uuid.UUID
	mailer Mailer
}

func (s *testState) restart() testState {
	s.listener.Close()
	s.emails.Close()
	s.alreadyClosed = true
	return prepare(prepareOpts{
		t:  s.t,
		id: s.id,
	})
}

func prepare(opts prepareOpts) testState {
	logger := zerolog.Nop()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	tmust(opts.t, err)

	id := opts.id
	if id == uuid.Nil {
		id = uuid.New()
	}

	mailer := opts.mailer
	if mailer == nil {
		mailer = DontSendMailer{t: opts.t}
	}

	os.Mkdir("test_data", 0755)
	emails, err := NewEmailDB("test_data/" + id.String() + ".txt")
	tmust(opts.t, err)

	router := Bootstrap(BootstrapOpts{
		Emails:  emails,
		Logger:  logger,
		Fetcher: DummyFetcher{},
		Mailer:  mailer,
	})

	go http.Serve(listener, router)

	return testState{
		t:        opts.t,
		id:       id,
		listener: listener,
		emails:   emails,
	}
}

func TestFetchRate(t *testing.T) {
	state := prepare(prepareOpts{t: t})
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
	state := prepare(prepareOpts{t: t})
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
	state1 := prepare(prepareOpts{t: t})
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

func TestSendEmails(t *testing.T) {
	var asserter func(to []string, subject string, body string)

	state := prepare(prepareOpts{
		t: t,
		mailer: DummyMailer{
			t: t,
			assertFn: func(to []string, subject string, body string) error {
				if subject != "Bitcoin rate" {
					t.Fatalf("Expected subject to be %q, got %q", "Bitcoin rate", subject)
				}
				if body != "Bitcoin rate is 42 USD" {
					t.Fatalf("Expected body to be %q, got %q", "Bitcoin rate is 42 USD", body)
				}
				asserter(to, subject, body)
				return nil
			},
		},
	})
    defer state.Close()

    buffer := bytes.NewBufferString("email=foo@mail.com")
    resp, err := http.Post(state.APIURL("/subscribe"), "application/x-www-form-urlencoded", buffer)
    tmust(t, err)

    if resp.StatusCode != 200 {
        t.Fatalf("Expected 200, got %d", resp.StatusCode)
    }

	asserter = func(to []string, subject string, body string) {
		if len(to) != 1 {
			t.Fatalf("Expected 1 recipient, got %d", len(to))
		}
		if to[0] != "foo@mail.com" {
			t.Fatalf("Expected to send mail to foo@mail.com, instead only sent to %v", to)
		}
	}

    resp, err = http.Post(state.APIURL("/sendEmails"), "", nil)
    tmust(t, err)

    if resp.StatusCode != 200 {
        t.Fatalf("Expected 200, got %d", resp.StatusCode)
    }

    buffer.Reset()
    buffer.WriteString("email=bar@my.notmail.org")
    resp, err = http.Post(state.APIURL("/subscribe"), "application/x-www-form-urlencoded", buffer)
    tmust(t, err)

    if resp.StatusCode != 200 {
        t.Fatalf("Expected 200, got %d", resp.StatusCode)
    }

	asserter = func(to []string, subject string, body string) {
		if len(to) != 2 {
			t.Fatalf("Expected 2 recipients, got %d", len(to))
		}
		if to[0] != "foo@mail.com" && to[1] != "foo@mail.com" {
			t.Fatalf("Expected to send mail to foo@mail.com, instead only sent to %v", to)
		}
		if to[0] != "bar@my.notmail.org" && to[1] != "bar@my.notmail.org" {
			t.Fatalf("Expected to send mail to bar@my.notmail.org, instead only sent to %v", to)
		}
	}

    resp, err = http.Post(state.APIURL("/sendEmails"), "", nil)
    tmust(t, err)

    if resp.StatusCode != 200 {
        t.Fatalf("Expected 200, got %d", resp.StatusCode)
    }

    buffer.Reset()
    buffer.WriteString("email=foo@mail.com")
    resp, err = http.Post(state.APIURL("/subscribe"), "application/x-www-form-urlencoded", buffer)
    tmust(t, err)

    if resp.StatusCode != 400 {
        t.Fatalf("Expected 400, got %d", resp.StatusCode)
    }

    resp, err = http.Post(state.APIURL("/sendEmails"), "", nil)
    tmust(t, err)

    if resp.StatusCode != 200 {
        t.Fatalf("Expected 200, got %d", resp.StatusCode)
    }
}
