package monitor

import (
 "os"
 "path/filepath"
 "testing"
)

func TestSMTPConfig(t *testing.T) {
 path := filepath.Join(t.TempDir(), "monitor.json")
 content := `{"providers":{"mail":{"type":"smtp","host":"smtp.example.com","port":587,"username_env":"SMTP_USER","password_env":"SMTP_PASS","from":"sender@example.com","to":["recipient@example.com"]}},"watches":[{"name":"home","source":"funda","city":"utrecht","category":"rent","notify":["mail"]}]}`
 if err := os.WriteFile(path, []byte(content), 0600); err != nil { t.Fatal(err) }
 c, err := Load(path)
 if err != nil { t.Fatal(err) }
 if c.Providers["mail"].Type != "smtp" { t.Fatal("SMTP provider missing") }
}

func TestSMTPInvalidPort(t *testing.T) {
 path := filepath.Join(t.TempDir(), "monitor.json")
 content := `{"providers":{"mail":{"type":"smtp","host":"smtp.example.com","port":25,"from":"sender@example.com","to":["recipient@example.com"]}}}`
 if err := os.WriteFile(path, []byte(content), 0600); err != nil { t.Fatal(err) }
 if _, err := Load(path); err == nil { t.Fatal("expected unsupported SMTP port error") }
}
