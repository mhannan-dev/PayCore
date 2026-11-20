package services

import (
	"errors"
	"net/textproto"
)

// Invitation delivery statuses mirrored by the existing API contract.
const (
	EmailPending       = "pending"
	EmailSent          = "sent"
	EmailFailed        = "failed"
	EmailNotConfigured = "not_configured"
)

// permanent reports SMTP 5xx replies, which retrying will not fix.
func permanent(err error) bool {
	var reply *textproto.Error
	return errors.As(err, &reply) && reply.Code >= 500 && reply.Code < 600
}

// emailLayout is the HTML layout for single-link emails (password reset). Fields:
// Title, Body, Button, Link, Expires, Footer — all escaped by html/template.
const emailLayout = `<!doctype html>
<html><body style="margin:0;background:#f6f7f4;font-family:Arial,Helvetica,sans-serif;color:#20362d">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:32px 16px">
<table role="presentation" width="100%" style="max-width:520px;background:#ffffff;border:1px solid #e4e9e3;border-radius:10px" cellpadding="0" cellspacing="0">
<tr><td style="padding:28px 32px 8px"><span style="display:inline-block;background:#245f43;color:#fff;font-weight:700;border-radius:8px;padding:6px 10px">p·</span>
<strong style="font-size:18px;margin-left:8px">PayCore</strong></td></tr>
<tr><td style="padding:12px 32px 0"><h1 style="font-size:20px;margin:0 0 12px">{{.Title}}</h1>
<p style="font-size:14px;line-height:1.6;margin:0 0 20px">{{.Body}}</p>
<p style="margin:0 0 24px"><a href="{{.Link}}" style="display:inline-block;background:#245f43;color:#fff;text-decoration:none;font-weight:600;font-size:14px;padding:12px 20px;border-radius:6px">{{.Button}}</a></p>
<p style="font-size:12px;line-height:1.6;color:#728078;margin:0 0 8px">This link works once and expires {{.Expires}}. If the button does not work, paste this address into your browser:</p>
<p style="font-size:12px;word-break:break-all;color:#245f43;margin:0 0 24px">{{.Link}}</p>
<p style="font-size:12px;color:#728078;margin:0 0 28px">{{.Footer}}</p></td></tr>
</table></td></tr></table></body></html>`
