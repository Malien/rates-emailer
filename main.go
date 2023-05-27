package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	chi "github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog"
	"github.com/rs/zerolog"
)

type CoinGeckoResponse struct {
	Bitcoin struct {
		Usd float64 `json:"usd"`
	} `json:"bitcoin"`
}

func rate(w http.ResponseWriter, r *http.Request) {
	oplog := httplog.LogEntry(r.Context())
	fetcher := Fetcher(r.Context())

	oplog.Info().Msg("Fetching excahnge rates")
	rate, err := fetcher.FetchRate(r.Context())
	if err != nil {
		oplog.Error().Err(err).Msg("Failed to fetch exchange rates")
		rateFetchFail(w)
		return
	}
	oplog.Info().Float64("rate", rate).Msg("Exchange rates fetched successfully")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	body, err := json.Marshal(rate)
	if err != nil {
		oplog.Error().Err(err).Float64("body", rate).Msg("Failed to marshal response body")
		rateFetchFail(w)
		return
	}
	w.Write(body)
}

func rateFetchFail(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "{\"error\": \"Failed to fetch exchange rates\"}")
}

func subscribe(w http.ResponseWriter, r *http.Request) {
	oplog := httplog.LogEntry(r.Context())
    db := EmailsDBFromContext(r.Context())

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

type AppConfig struct {
    Emails *EmailDB
    Logger zerolog.Logger
    Fetcher RateFetcher
}

func Bootstrap(config AppConfig) chi.Router {
	router := chi.NewRouter()
	router.Use(httplog.RequestLogger(config.Logger))
    router.Use(attachValue(FetcherCtxKey, config.Fetcher))
    router.Use(attachValue(EmailDBCtxKey, config.Emails))

	router.Get("/rate", rate)
	router.Post("/subscribe", subscribe)

    return router
}

func attachValue(key interface{}, value interface{}) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), key, value)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func main() {
	logger := httplog.NewLogger("genesis-case", httplog.Options{
		// JSON: true,
	})

	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to listen")
		return
	}
	logger.Info().Msg("Listening on :8080")

    emails, err := NewEmailDB("emails.txt")
    if err != nil {
        logger.Fatal().Err(err).Msg("Failed to read emails")
        return
    }

    router := Bootstrap(AppConfig {
        Emails: emails,
        Logger: logger,
        Fetcher: GeckoAPI{},
    })

	logger.Fatal().Err(http.Serve(listener, router))
}
