package email

import (
	"fmt"
	"net/smtp"
	"strings"

	"github.com/yhzion/claude-postman/internal/config"
)

// SMTPSender abstracts SMTP sending for testability.
type SMTPSender interface {
	Send(from, to, subject, htmlBody, messageID, inReplyTo string) error
}

// smtpSender is the real SMTP implementation using net/smtp.
type smtpSender struct {
	host     string
	port     int
	user     string
	password string
}

func newSMTPSender(cfg *config.EmailConfig) SMTPSender {
	return &smtpSender{
		host:     cfg.SMTPHost,
		port:     cfg.SMTPPort,
		user:     cfg.User,
		password: cfg.AppPassword,
	}
}

func (s *smtpSender) Send(from, to, subject, htmlBody, messageID, inReplyTo string) error {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	if messageID != "" {
		b.WriteString("Message-ID: " + messageID + "\r\n")
	}
	if inReplyTo != "" {
		b.WriteString("In-Reply-To: " + inReplyTo + "\r\n")
		b.WriteString("References: " + inReplyTo + "\r\n")
	}
	b.WriteString("\r\n")
	b.WriteString(htmlBody)

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	auth := smtp.PlainAuth("", s.user, s.password, s.host)
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(b.String()))
}
