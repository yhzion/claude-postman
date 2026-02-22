package email

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	gomessage "github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	"github.com/yhzion/claude-postman/internal/config"
)

// RawEmail holds the parsed fields from a fetched IMAP message.
type RawEmail struct {
	From       string
	Subject    string
	Body       string
	MessageID  string
	InReplyTo  string
	References []string
	UID        imap.UID
}

// IMAPClient abstracts IMAP operations for testability.
type IMAPClient interface {
	FetchUnread(subject string) ([]*RawEmail, error)
	MarkRead(uid imap.UID) error
	Close() error
}

// imapClient is the real IMAP implementation using emersion/go-imap v2.
type imapClient struct {
	client *imapclient.Client
}

func newIMAPClient(cfg *config.EmailConfig) (IMAPClient, error) {
	addr := fmt.Sprintf("%s:%d", cfg.IMAPHost, cfg.IMAPPort)
	c, err := imapclient.DialTLS(addr, nil)
	if err != nil {
		return nil, fmt.Errorf("imap dial: %w", err)
	}
	if err := c.Login(cfg.User, cfg.AppPassword).Wait(); err != nil {
		c.Close()
		return nil, fmt.Errorf("imap login: %w", err)
	}
	if _, err := c.Select("INBOX", nil).Wait(); err != nil {
		c.Close()
		return nil, fmt.Errorf("imap select: %w", err)
	}
	return &imapClient{client: c}, nil
}

func (ic *imapClient) FetchUnread(subject string) ([]*RawEmail, error) {
	criteria := &imap.SearchCriteria{
		NotFlag: []imap.Flag{imap.FlagSeen},
		Header: []imap.SearchCriteriaHeaderField{
			{Key: "SUBJECT", Value: subject},
		},
	}
	searchData, err := ic.client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("imap search: %w", err)
	}
	if len(searchData.AllUIDs()) == 0 {
		return nil, nil
	}

	uidSet := imap.UIDSetNum(searchData.AllUIDs()...)
	fetchOptions := &imap.FetchOptions{
		UID:      true,
		Envelope: true,
		BodySection: []*imap.FetchItemBodySection{
			{Specifier: imap.PartSpecifierNone},
		},
	}

	buffers, err := ic.client.Fetch(uidSet, fetchOptions).Collect()
	if err != nil {
		return nil, fmt.Errorf("imap fetch: %w", err)
	}

	var emails []*RawEmail
	for _, buf := range buffers {
		raw := bufferToRawEmail(buf)
		emails = append(emails, raw)
	}
	return emails, nil
}

func (ic *imapClient) MarkRead(uid imap.UID) error {
	uidSet := imap.UIDSetNum(uid)
	storeFlags := &imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagSeen},
	}
	return ic.client.Store(uidSet, storeFlags, nil).Close()
}

func (ic *imapClient) Close() error {
	return ic.client.Close()
}

func bufferToRawEmail(buf *imapclient.FetchMessageBuffer) *RawEmail {
	raw := &RawEmail{UID: buf.UID}

	// Extract envelope data
	if env := buf.Envelope; env != nil {
		raw.Subject = env.Subject
		raw.MessageID = env.MessageID
		if len(env.InReplyTo) > 0 {
			raw.InReplyTo = env.InReplyTo[0]
		}
		if len(env.From) > 0 {
			raw.From = env.From[0].Addr()
		}
	}

	// Extract body and References header from body section
	bodySection := &imap.FetchItemBodySection{Specifier: imap.PartSpecifierNone}
	if data := buf.FindBodySection(bodySection); data != nil {
		raw.Body, raw.References = parseEmailBody(bytes.NewReader(data))
	}

	return raw
}

func parseEmailBody(r io.Reader) (body string, refs []string) {
	gomessage.CharsetReader = nil
	mr, err := mail.CreateReader(r)
	if err != nil {
		return "", nil
	}
	defer mr.Close()

	// Get References from header
	if refHeader, err := mr.Header.Text("References"); err == nil && refHeader != "" {
		refs = strings.Fields(refHeader)
	}

	// Read body parts
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		if _, ok := p.Header.(*mail.InlineHeader); ok {
			b, err := io.ReadAll(p.Body)
			if err == nil {
				body = string(b)
			}
		}
	}
	return
}
