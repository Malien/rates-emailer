package main

import (
	"fmt"
	"net"
	"net/http"

	chi "github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog"
	"github.com/rs/zerolog"
)

func rate(w http.ResponseWriter, r *http.Request) {
	oplog := httplog.LogEntry(r.Context())
	oplog.Info().Msg("Fetching excahnge rates")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// Numbers are valid JSON
	fmt.Fprintf(w, "%d", 42)
}

func subscribe(w http.ResponseWriter, r *http.Request) {
	oplog := httplog.LogEntry(r.Context())
	email := r.FormValue("email")
	if email == "" {
		oplog.Error().Msg("Email not provided")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "{\"error\": \"Email is required\"}")
		return
	}
    // Arbitrary length restriction. It couldn't be more than 64K tho
    if len(email) >= 512 {
        oplog.Error().Msg("Email address too long")
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusBadRequest)
        fmt.Fprintf(w, "{\"errror\": \"Email cannot be longer than 512 characters\"}")
        return
    }
	oplog = oplog.With().Str("email", email).Logger()

    oplog.Info().Msg("Saving subscriber to the file")

    exists, err := db.Append(email)

    if err != nil {
        oplog.Error().Err(err).Msg("Failed to save subscriber")
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusInternalServerError)
        fmt.Fprintf(w, "{\"error\": \"Failed to save subscriber\"}")
        return
    }

    if exists {
        oplog.Info().Msg("Subscriber already exists")
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusBadRequest)
        fmt.Fprintf(w, "{\"error\": \"Subscriber already exists\"}")
        return
    }

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

func router(logger zerolog.Logger) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(httplog.RequestLogger(logger))

	r.Get("/rate", rate)
	r.Post("/subscribe", subscribe)

	return r
}

var db *EmailDB

func main() {
	logger := httplog.NewLogger("genesis-case", httplog.Options{
		// JSON: true,
	})

    var err error
    db, err = NewEmailDB("emails.txt")
    if err != nil {
        logger.Fatal().Err(err).Msg("Couldn't read persisted emails")
    }

	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to listen")
		return
	}

	logger.Info().Msg("Listening on :8080")

	logger.Fatal().Err(http.Serve(listener, router(logger)))
}
