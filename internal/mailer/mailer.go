package mailer

import (
	"fmt"

	mail "github.com/wneessen/go-mail"

	"github.com/axlsoft/lantern/internal/config"
)

// Sender is the interface that auth and org handlers depend on.
type Sender interface {
	SendVerification(to, token string) error
	SendPasswordReset(to, token string) error
	SendOrgInvite(to, orgName, token string) error
}

// Mailer sends transactional emails via SMTP.
type Mailer struct {
	cfg    *config.Config
	client *mail.Client
}

// New creates a Mailer connected to the configured SMTP server.
func New(cfg *config.Config) (*Mailer, error) {
	opts := []mail.Option{
		mail.WithPort(cfg.SMTPPort),
		mail.WithTLSPolicy(mail.NoTLS),
	}
	if cfg.SMTPUsername != "" {
		opts = append(opts, mail.WithUsername(cfg.SMTPUsername), mail.WithPassword(cfg.SMTPPassword))
	}
	client, err := mail.NewClient(cfg.SMTPHost, opts...)
	if err != nil {
		return nil, fmt.Errorf("new mail client: %w", err)
	}
	return &Mailer{cfg: cfg, client: client}, nil
}

func (m *Mailer) SendVerification(to, token string) error {
	link := fmt.Sprintf("%s/api/v1/auth/verify?token=%s", m.cfg.BaseURL, token)
	return m.send(to, "Verify your Lantern account", fmt.Sprintf(
		"Click the link below to verify your email address:\n\n%s\n\nThis link expires in 24 hours.", link,
	))
}

func (m *Mailer) SendPasswordReset(to, token string) error {
	link := fmt.Sprintf("%s/reset-password?token=%s", m.cfg.BaseURL, token)
	return m.send(to, "Reset your Lantern password", fmt.Sprintf(
		"Click the link below to reset your password:\n\n%s\n\nThis link expires in 1 hour.", link,
	))
}

func (m *Mailer) SendOrgInvite(to, orgName, token string) error {
	link := fmt.Sprintf("%s/invites/%s/accept", m.cfg.BaseURL, token)
	return m.send(to, fmt.Sprintf("You've been invited to %s on Lantern", orgName), fmt.Sprintf(
		"You have been invited to join %s on Lantern.\n\nAccept your invitation:\n%s\n\nThis link expires in 7 days.", orgName, link,
	))
}

func (m *Mailer) send(to, subject, body string) error {
	msg := mail.NewMsg()
	if err := msg.From(m.cfg.EmailFrom); err != nil {
		return err
	}
	if err := msg.To(to); err != nil {
		return err
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, body)
	return m.client.DialAndSend(msg)
}
