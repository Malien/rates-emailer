package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/mail"
	"os"
	"strconv"

	chi "github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

type CoinGeckoResponse struct {
	Bitcoin struct {
		Usd float64 `json:"usd"`
	} `json:"bitcoin"`
}

func rate(w http.ResponseWriter, r *http.Request) {
	oplog := httplog.LogEntry(r.Context())
	fetcher := FetcherFromContext(r.Context())

	oplog.Info().Msg("Fetching excahnge rates")
	rate, err := fetcher.FetchRate(r.Context())
	if err != nil {
		oplog.Error().Err(err).Msg("Failed to fetch exchange rates")
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusInternalServerError)
        fmt.Fprintf(w, "{\"error\": \"Failed to fetch exchange rates\"}")
		return
	}
	oplog.Info().Float64("rate", rate).Msg("Exchange rates fetched successfully")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	body, err := json.Marshal(rate)
	if err != nil {
		oplog.Error().Err(err).Float64("body", rate).Msg("Failed to marshal response body")
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusInternalServerError)
        fmt.Fprintf(w, "{\"error\": \"Failed to serialize response body\"}")
		return
	}
	w.Write(body)
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
	_, err := mail.ParseAddress(email)
	if err != nil {
		oplog.Error().Err(err).Msg("Invalid email address")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "{\"error\": \"Invalid email address\"}")
		return
	}

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

func sendEmails(w http.ResponseWriter, r *http.Request) {
	oplog := httplog.LogEntry(r.Context())
	mailer := MailerFromContext(r.Context())
	emails := EmailsDBFromContext(r.Context())
	fetcher := FetcherFromContext(r.Context())

	oplog.Info().Msg("Fetching exchange rate...")
	rate, err := fetcher.FetchRate(r.Context())
	if err != nil {
		oplog.Error().Err(err).Msg("Failed to fetch exchange rate")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "{\"error\": \"Failed to fetch exchange rate\"}")
		return
	}
	oplog.Info().Float64("rate", rate).Msg("Exchange rate fetched successfully")

	oplog.Info().Msg("Sending email...")
	body := "Bitcoin rate is " + strconv.FormatFloat(rate, 'f', -1, 64) + " USD"
	err = mailer.Send(r.Context(), emails.Emails(), "Bitcoin rate", body)
	if err != nil {
		oplog.Error().Err(err).Msg("Failed to send email")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "{\"error\": \"Failed to send email\"}")
		return
	}
	oplog.Info().Msg("Sent " + strconv.Itoa(len(emails.Emails())) + " emails")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

type EmailConfig struct {
	Username string
	Password string
	From     string
	Smtp     struct {
		Host string
		Port int
		SSL  bool
	}
}

type AppConfig struct {
	Email          EmailConfig
	Bind           string
	EmailsFilePath string
	UseJSONLogs    bool
}

type BootstrapOpts struct {
	Emails  *EmailDB
	Logger  zerolog.Logger
	Fetcher RateFetcher
	Mailer  Mailer
}

func Bootstrap(config BootstrapOpts) chi.Router {
	router := chi.NewRouter()
	router.Use(httplog.RequestLogger(config.Logger))
	router.Use(attachValue(FetcherCtxKey, config.Fetcher))
	router.Use(attachValue(EmailDBCtxKey, config.Emails))
	router.Use(attachValue(MailerCtxKey, config.Mailer))

	router.Get("/rate", rate)
	router.Post("/subscribe", subscribe)
	router.Post("/sendEmails", sendEmails)

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
	mode := os.Getenv("APP_MODE")
	if mode == "" {
		mode = "dev"
	}

	// Load config in order:
	// - config.*
	// - config.(dev|prod).*
	// - config.local.*
	// - Environment variables
	// * refers to (json|toml|yaml|yml)

	viper.SetConfigName("config")
	viper.AddConfigPath("./conf")
	viper.ReadInConfig()

	viper.SetConfigName("config." + mode)
	viper.MergeInConfig()

	viper.SetConfigName("config.local")
	viper.MergeInConfig()

	viper.AutomaticEnv()
	viper.BindEnv("email.username", "EMAIL_USERNAME")
	viper.BindEnv("email.password", "EMAIL_PASSWORD")
	viper.BindEnv("email.from", "EMAIL_FROM")
	viper.BindEnv("email.smtp.host", "EMAIL_SMTP_HOST")
	viper.MergeInConfig()

	var config *AppConfig
	err := viper.Unmarshal(&config)
	if err != nil {
        log.Fatalf("Failed to parse config file: %s", err)
	}

	logger := httplog.NewLogger("rates-emailer", httplog.Options{
		JSON: config.UseJSONLogs,
	})

	logger.Info().Msg("Reading emails from " + config.EmailsFilePath)
	emails, err := NewEmailDB(config.EmailsFilePath)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to read emails")
	}

	listener, err := net.Listen("tcp", config.Bind)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to listen")
	}
	logger.Info().Msg("Listening on " + config.Bind)

	mailer, err := NewSmtpMailer(&config.Email)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create mailer")
	}

	router := Bootstrap(BootstrapOpts{
		Emails:  emails,
		Logger:  logger,
		Fetcher: GeckoAPI{},
		Mailer:  mailer,
	})

	logger.Fatal().Err(http.Serve(listener, router))
}
