# Watch preferences and SMTP email notifications

The monitoring configuration is JSON. The included example is `examples/monitor.json`.

Show your saved searches and notification destination names (without printing secrets):

```sh
funda watch list --config examples/monitor.json
```

Edit `watches` to set `city`, `category`, `max_price`, `min_area`, `min_bedrooms`, and the list of destination names under `notify`. The sample email destination `email_personal` uses SMTP with TLS. Replace its `from` and `to` addresses with real addresses, and adjust the SMTP host and port to your provider. Ports 587 (STARTTLS) and 465 (implicit TLS) are supported. SMTP authentication is required.

```sh
export SMTP_USERNAME='your-email@gmail.com'
export SMTP_PASSWORD='your-app-password'
funda notify test --config examples/monitor.json --provider email_personal
funda check --config examples/monitor.json
```

For Gmail, use an app password where available; your normal Google account password generally will not work for SMTP. Never commit credentials to source control. `funda check` fetches and enriches listings before matching and sending alerts. `funda watch` repeats the same operation on the configured interval.
