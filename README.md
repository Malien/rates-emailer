# rates-emailer
A small Go service to query and subscribe to BTC to USD rates

In order to make the mailer do the mails, the following configuration parameters have to be supplied, either in `/conf/config.local.(toml|yml|json)` or as an environment variables. Be careful not to commit secrets.
- `EMAIL_SMTP_HOST` — a hostname for the smtp server, that will be used to send out emails
- `EMAIL_USERNAME` — username for authorization to the said smtp server
- `EMAIL_PASSWORD` — the task of figuring this out I'll leave as an excercise to the reader
- `EMAIL_FROM` — from whom should be emails sent. Must be a valid RFC(_something, something_) email address
- `APP_MODE` — can be set to `dev` or `prod` (`dev` by default). Changes which set of configs will be loaded. Either `/conf/config.dev.(toml|yml|json)` or `/conf/config.prod.(toml|yml|json)`. 

go do the `go run .` thing to your heart's content
