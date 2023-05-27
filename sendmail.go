package main

import (
	"context"

	"github.com/go-chi/httplog"
	mail "github.com/wneessen/go-mail"
)

type SmtpMailer struct {
	client *mail.Client
	from   string
}

func NewSmtpMailer(config *EmailConfig) (mailer SmtpMailer, err error) {
	mailer.from = config.From
	mailer.client, err = mail.NewClient(
		config.Smtp.Host,
		mail.WithPort(config.Smtp.Port),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(config.Username),
		mail.WithPassword(config.Password),
	)
	if err != nil {
		return
	}

	if config.Smtp.SSL {
		err = mail.WithSSL()(mailer.client)
		if err != nil {
			return
		}
	}

	return
}

type Mailer interface {
	Send(ctx context.Context, to []string, subject string, body string) error
}

func (m SmtpMailer) Send(ctx context.Context, to []string, subject string, body string) error {
	messages := make([]*mail.Msg, 0, len(to))
	logger := httplog.LogEntry(ctx)

	for _, recipient := range to {
		msg := mail.NewMsg()
		err := msg.From(m.from)
		if err != nil {
			return err
		}
		msg.Subject(subject)
		msg.SetBodyString(mail.TypeTextPlain, body)
		err = msg.To(recipient)
		if err != nil {
			logger.
                Warn().
				Err(err).
				Str("recipient", recipient).
				Msg("Invalid email address")
			continue
		}

		messages = append(messages, msg)
	}

	return m.client.DialAndSendWithContext(ctx, messages...)
}

type mailerKey int

var MailerCtxKey mailerKey

func MailerFromContext(ctx context.Context) Mailer {
	return ctx.Value(MailerCtxKey).(Mailer)
}
