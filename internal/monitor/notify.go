package monitor

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"strings"
	"time"
)

func Send(ctx context.Context, p Provider, message string) error {
	if p.Type == "smtp" {
		return sendSMTP(ctx, p, message)
	}
	var endpoint string
	var payload any
	switch p.Type {
	case "telegram":
		token, chat := os.Getenv(p.TokenEnv), os.Getenv(p.ChatIDEnv)
		if token == "" || chat == "" {
			return fmt.Errorf("telegram credentials not set")
		}
		endpoint = "https://api.telegram.org/bot" + url.PathEscape(token) + "/sendMessage"
		payload = map[string]string{"chat_id": chat, "text": message}
	case "webhook":
		endpoint = os.Getenv(p.URLEnv)
		u, err := url.Parse(endpoint)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return fmt.Errorf("webhook requires an HTTPS URL")
		}
		// Avoid accidentally sending credentials or listing data to localhost/private endpoints.
		host := strings.ToLower(u.Hostname())
		if host == "localhost" || strings.HasSuffix(host, ".local") {
			return fmt.Errorf("local webhook host rejected")
		}
		payload = map[string]string{"text": message}
	default:
		return fmt.Errorf("unknown notification provider %q", p.Type)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("notification HTTP %d", res.StatusCode)
	}
	return nil
}

// sendSMTP sends a plain-text email over authenticated TLS. Port 465 uses
// implicit TLS; port 587 requires STARTTLS. No plaintext fallback is allowed.
func sendSMTP(ctx context.Context, p Provider, message string) error {
	from, err := mail.ParseAddress(p.From)
	if err != nil {
		return fmt.Errorf("invalid sender: %w", err)
	}
	recipients := make([]string, 0, len(p.To))
	for _, address := range p.To {
		to, err := mail.ParseAddress(address)
		if err != nil {
			return fmt.Errorf("invalid recipient: %w", err)
		}
		recipients = append(recipients, to.Address)
	}
	user, pass := os.Getenv(p.UsernameEnv), os.Getenv(p.PasswordEnv)
	if user == "" || pass == "" {
		return fmt.Errorf("SMTP credentials not set")
	}
	address := net.JoinHostPort(p.Host, fmt.Sprint(p.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	tlsConfig := &tls.Config{ServerName: p.Host, MinVersion: tls.VersionTLS12}
	if p.Port == 465 {
		secure := tls.Client(conn, tlsConfig)
		if err := secure.HandshakeContext(ctx); err != nil {
			return err
		}
		conn = secure
	}
	client, err := smtp.NewClient(conn, p.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if p.Port == 587 {
		ok, _ := client.Extension("STARTTLS")
		if !ok {
			return fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if err := client.Auth(smtp.PlainAuth("", user, pass, p.Host)); err != nil {
		return err
	}
	if err := client.Mail(from.Address); err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	// Prevent header injection: headers use only validated email addresses.
	body := strings.ReplaceAll(strings.ReplaceAll(message, "\r\n", "\n"), "\n", "\r\n")
	payload := "From: " + from.Address + "\r\nTo: " + strings.Join(recipients, ", ") + "\r\nSubject: funda-cli property alert\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body + "\r\n"
	if _, err := writer.Write([]byte(payload)); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
