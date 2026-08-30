package alert

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Telegram TelegramConfig `yaml:"telegram,omitempty" json:"telegram"`
	SMTP     SMTPConfig     `yaml:"smtp,omitempty" json:"smtp"`
	Monitor  MonitorConfig  `yaml:"monitor,omitempty" json:"monitor"`
}

type MonitorConfig struct {
	Enabled                bool     `yaml:"enabled,omitempty" json:"enabled"`
	Schedule               string   `yaml:"schedule,omitempty" json:"schedule,omitempty"`
	CPUWarningPercent      int      `yaml:"cpuWarningPercent,omitempty" json:"cpuWarningPercent,omitempty"`
	CPUCriticalPercent     int      `yaml:"cpuCriticalPercent,omitempty" json:"cpuCriticalPercent,omitempty"`
	MemoryWarningPercent   int      `yaml:"memoryWarningPercent,omitempty" json:"memoryWarningPercent,omitempty"`
	MemoryCriticalPercent  int      `yaml:"memoryCriticalPercent,omitempty" json:"memoryCriticalPercent,omitempty"`
	DiskWarningPercent     int      `yaml:"diskWarningPercent,omitempty" json:"diskWarningPercent,omitempty"`
	DiskCriticalPercent    int      `yaml:"diskCriticalPercent,omitempty" json:"diskCriticalPercent,omitempty"`
	CertificateWarningDays int      `yaml:"certificateWarningDays,omitempty" json:"certificateWarningDays,omitempty"`
	Services               []string `yaml:"services,omitempty" json:"services,omitempty"`
	CertificateRoots       []string `yaml:"certificateRoots,omitempty" json:"certificateRoots,omitempty"`
}

type TelegramConfig struct {
	Enabled   bool   `yaml:"enabled,omitempty" json:"enabled"`
	TokenFile string `yaml:"tokenFile,omitempty" json:"tokenFile,omitempty"`
	ChatID    string `yaml:"chatId,omitempty" json:"chatId,omitempty"`
}

type SMTPConfig struct {
	Enabled      bool     `yaml:"enabled,omitempty" json:"enabled"`
	Host         string   `yaml:"host,omitempty" json:"host,omitempty"`
	Port         int      `yaml:"port,omitempty" json:"port,omitempty"`
	Username     string   `yaml:"username,omitempty" json:"username,omitempty"`
	PasswordFile string   `yaml:"passwordFile,omitempty" json:"passwordFile,omitempty"`
	From         string   `yaml:"from,omitempty" json:"from,omitempty"`
	To           []string `yaml:"to,omitempty" json:"to,omitempty"`
	TLS          string   `yaml:"tls,omitempty" json:"tls,omitempty"`
}

func Load(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("could not read alert configuration: %w", err)
	}
	var config Config
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("could not decode alert configuration: %w", err)
	}
	config.Monitor = withMonitorDefaults(config.Monitor)
	if err := Validate(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func Validate(config Config) error {
	if config.Telegram.Enabled && (config.Telegram.TokenFile == "" || config.Telegram.ChatID == "") {
		return fmt.Errorf("enabled Telegram alerts require tokenFile and chatId")
	}
	if config.SMTP.Enabled {
		if config.SMTP.Host == "" || config.SMTP.Port < 1 || config.SMTP.Port > 65535 || config.SMTP.From == "" || len(config.SMTP.To) == 0 {
			return fmt.Errorf("enabled SMTP alerts require host, a valid port, from, and at least one recipient")
		}
		if config.SMTP.TLS == "" {
			config.SMTP.TLS = "starttls"
		}
		if config.SMTP.TLS != "starttls" && config.SMTP.TLS != "implicit" {
			return fmt.Errorf("SMTP tls must be starttls or implicit")
		}
	}
	monitor := withMonitorDefaults(config.Monitor)
	if monitor.DiskWarningPercent < 1 || monitor.DiskWarningPercent > 99 || monitor.DiskCriticalPercent < 1 || monitor.DiskCriticalPercent > 100 || monitor.DiskWarningPercent >= monitor.DiskCriticalPercent {
		return fmt.Errorf("monitor disk thresholds must be between 1 and 100, with warning below critical")
	}
	if err := validateMonitorThresholds("CPU", monitor.CPUWarningPercent, monitor.CPUCriticalPercent); err != nil {
		return err
	}
	if err := validateMonitorThresholds("memory", monitor.MemoryWarningPercent, monitor.MemoryCriticalPercent); err != nil {
		return err
	}
	if monitor.CertificateWarningDays < 1 {
		return fmt.Errorf("monitor certificateWarningDays must be at least 1")
	}
	return nil
}

func withMonitorDefaults(config MonitorConfig) MonitorConfig {
	if config.Schedule == "" {
		config.Schedule = "*-*-* *:00/15:00"
	}
	if config.DiskWarningPercent == 0 {
		config.DiskWarningPercent = 80
	}
	if config.DiskCriticalPercent == 0 {
		config.DiskCriticalPercent = 90
	}
	if config.CPUWarningPercent == 0 {
		config.CPUWarningPercent = 80
	}
	if config.CPUCriticalPercent == 0 {
		config.CPUCriticalPercent = 95
	}
	if config.MemoryWarningPercent == 0 {
		config.MemoryWarningPercent = 80
	}
	if config.MemoryCriticalPercent == 0 {
		config.MemoryCriticalPercent = 90
	}
	if config.CertificateWarningDays == 0 {
		config.CertificateWarningDays = 30
	}
	return config
}

func validateMonitorThresholds(name string, warning, critical int) error {
	if warning < 1 || warning > 99 || critical < 1 || critical > 100 || warning >= critical {
		return fmt.Errorf("monitor %s thresholds must be between 1 and 100, with warning below critical", name)
	}
	return nil
}

type MonitorIssue struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type MonitorState struct {
	Issues map[string]string `json:"issues"`
}

type MonitorDelta struct {
	NewIssues []MonitorIssue
	Resolved  []string
	NextState MonitorState
}

func CompareMonitorState(previous MonitorState, current []MonitorIssue) MonitorDelta {
	if previous.Issues == nil {
		previous.Issues = map[string]string{}
	}
	next := MonitorState{Issues: map[string]string{}}
	delta := MonitorDelta{NextState: next}
	for _, issue := range current {
		if issue.Status == "pass" {
			continue
		}
		fingerprint := monitorFingerprint(issue)
		delta.NextState.Issues[issue.Name] = fingerprint
		if previous.Issues[issue.Name] != fingerprint {
			delta.NewIssues = append(delta.NewIssues, issue)
		}
	}
	for name := range previous.Issues {
		if _, active := delta.NextState.Issues[name]; !active {
			delta.Resolved = append(delta.Resolved, name)
		}
	}
	sort.Strings(delta.Resolved)
	sort.Slice(delta.NewIssues, func(i, j int) bool { return delta.NewIssues[i].Name < delta.NewIssues[j].Name })
	return delta
}

func monitorFingerprint(issue MonitorIssue) string {
	hash := sha256.Sum256([]byte(issue.Status + "\x00" + issue.Message))
	return hex.EncodeToString(hash[:])
}

func LoadMonitorState(path string) (MonitorState, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return MonitorState{Issues: map[string]string{}}, nil
	}
	if err != nil {
		return MonitorState{}, fmt.Errorf("could not read monitor state: %w", err)
	}
	var state MonitorState
	if err := json.Unmarshal(content, &state); err != nil {
		return MonitorState{}, fmt.Errorf("could not decode monitor state: %w", err)
	}
	if state.Issues == nil {
		state.Issues = map[string]string{}
	}
	return state, nil
}

func SaveMonitorState(path string, state MonitorState) error {
	if state.Issues == nil {
		state.Issues = map[string]string{}
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode monitor state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("could not create monitor state directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".omurga-alert-state-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(content, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("could not replace monitor state: %w", err)
	}
	return nil
}

func Send(ctx context.Context, config Config, channel, subject, message string) error {
	var failures []error
	if (channel == "all" || channel == "telegram") && config.Telegram.Enabled {
		if err := sendTelegram(ctx, config.Telegram, message); err != nil {
			failures = append(failures, err)
		}
	}
	if (channel == "all" || channel == "email") && config.SMTP.Enabled {
		if err := sendSMTP(ctx, config.SMTP, subject, message); err != nil {
			failures = append(failures, err)
		}
	}
	if channel != "all" && channel != "telegram" && channel != "email" {
		return fmt.Errorf("alert channel must be all, telegram, or email")
	}
	if len(failures) == 0 && ((channel == "telegram" && !config.Telegram.Enabled) || (channel == "email" && !config.SMTP.Enabled) || (channel == "all" && !config.Telegram.Enabled && !config.SMTP.Enabled)) {
		return fmt.Errorf("no requested alert channel is enabled")
	}
	return errors.Join(failures...)
}

func sendTelegram(ctx context.Context, config TelegramConfig, message string) error {
	token, err := readCredential(config.TokenFile)
	if err != nil {
		return fmt.Errorf("could not read Telegram token: %w", err)
	}
	body, err := json.Marshal(map[string]string{"chat_id": config.ChatID, "text": message})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("Telegram request failed before a response was received")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Telegram API returned HTTP %d", response.StatusCode)
	}
	return nil
}

func sendSMTP(ctx context.Context, config SMTPConfig, subject, message string) error {
	address := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var connection net.Conn
	var err error
	if config.TLS == "implicit" {
		connection, err = tls.DialWithDialer(dialer, "tcp", address, &tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12})
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("could not connect to SMTP server: %w", err)
	}
	defer connection.Close()
	client, err := smtp.NewClient(connection, config.Host)
	if err != nil {
		return fmt.Errorf("could not initialize SMTP client: %w", err)
	}
	defer client.Close()
	if config.TLS != "implicit" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP server does not offer STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("could not enable SMTP TLS: %w", err)
		}
	}
	if config.Username != "" {
		password, err := readCredential(config.PasswordFile)
		if err != nil {
			return fmt.Errorf("could not read SMTP password: %w", err)
		}
		if err := client.Auth(smtp.PlainAuth("", config.Username, password, config.Host)); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}
	sender, err := mail.ParseAddress(config.From)
	if err != nil {
		return fmt.Errorf("invalid SMTP sender: %w", err)
	}
	if err := client.Mail(sender.Address); err != nil {
		return err
	}
	for _, recipient := range config.To {
		address, err := mail.ParseAddress(recipient)
		if err != nil {
			return fmt.Errorf("invalid SMTP recipient: %w", err)
		}
		if err := client.Rcpt(address.Address); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	payload := "From: " + config.From + "\r\nTo: " + strings.Join(config.To, ", ") + "\r\nSubject: " + sanitizeHeader(subject) + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + message + "\r\n"
	if _, err := writer.Write([]byte(payload)); err != nil {
		writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func readCredential(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("credential file path is required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(content))
	if value == "" {
		return "", fmt.Errorf("credential file is empty")
	}
	return value, nil
}

func sanitizeHeader(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}
