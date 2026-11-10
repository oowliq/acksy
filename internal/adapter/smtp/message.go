// Package smtp accepts email for publication as Acksy notifications.
package smtp

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oowliq/acksy/internal/domain"
)

const maxMessageBytes = 256 * 1024

func parseMessage(r io.Reader, topic string) (domain.Message, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxMessageBytes+1))
	if err != nil || len(raw) > maxMessageBytes {
		return domain.Message{}, domain.Invalid("invalid or oversized email")
	}
	email, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return domain.Message{}, domain.Invalid("invalid email headers")
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(email.Header.Get("Subject"))
	if err != nil || !utf8.ValidString(subject) || strings.ContainsRune(subject, 0) {
		return domain.Message{}, domain.Invalid("invalid email subject")
	}
	var plain plainBody
	if err := plain.read(textproto.MIMEHeader(email.Header), email.Body, 0); err != nil {
		return domain.Message{}, err
	}
	if !plain.found {
		return domain.Message{}, domain.Invalid("email requires a text/plain body")
	}
	msg := domain.Message{Topic: topic, Title: subject, Body: plain.text, Source: "smtp"}
	if err := msg.Normalize(time.Now()); err != nil {
		return domain.Message{}, err
	}
	return msg, nil
}

// plainBody selects the first inline text/plain part in wire order.
type plainBody struct {
	text  string
	found bool
	parts int
}

func (p *plainBody) read(h textproto.MIMEHeader, r io.Reader, depth int) error {
	p.parts++
	if depth > 10 || p.parts > 100 {
		return domain.Invalid("too many MIME parts or nesting levels")
	}
	if disposition := h.Get("Content-Disposition"); disposition != "" {
		kind, params, err := mime.ParseMediaType(disposition)
		if err != nil {
			return domain.Invalid("invalid content disposition")
		}
		if kind != "inline" || params["filename"] != "" {
			return nil
		}
	}
	contentType := h.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain"
	}
	kind, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return domain.Invalid("invalid MIME type")
	}
	if params["name"] != "" {
		return nil
	}
	encoding := strings.ToLower(h.Get("Content-Transfer-Encoding"))
	switch kind {
	case "multipart/alternative", "multipart/mixed", "multipart/related":
		if params["boundary"] == "" || (encoding != "" && encoding != "7bit" && encoding != "8bit" && encoding != "binary") {
			return domain.Invalid("invalid multipart envelope")
		}
		return p.readMultipart(multipart.NewReader(r, params["boundary"]), depth+1)
	case "text/plain":
		if p.found {
			return nil
		}
	default:
		return nil
	}
	switch strings.ToLower(params["charset"]) {
	case "", "utf-8", "us-ascii":
	default:
		return domain.Invalid("unsupported email charset")
	}
	switch encoding {
	case "", "7bit", "8bit", "binary":
	case "base64":
		r = base64.NewDecoder(base64.StdEncoding, r)
	case "quoted-printable":
		r = quotedprintable.NewReader(r)
	default:
		return domain.Invalid("unsupported transfer encoding")
	}
	b, err := io.ReadAll(r)
	if err != nil || !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		return domain.Invalid("invalid text/plain body")
	}
	p.text = strings.ReplaceAll(string(b), "\r\n", "\n")
	p.found = true
	return nil
}

func (p *plainBody) readMultipart(reader *multipart.Reader, depth int) error {
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF { // Only a clean final boundary terminates the message.
			return nil
		}
		if err != nil {
			return domain.Invalid("invalid multipart body")
		}
		if err := p.read(part.Header, part, depth); err != nil {
			return err
		}
	}
}
