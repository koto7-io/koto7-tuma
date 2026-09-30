package notification

import "context"

// ResolvingSender loads SMTP settings at send time so a change in Admin
// applies on the next alert without restarting the worker.
type ResolvingSender struct {
	resolve func(context.Context) (SMTPConfig, error)
}

func NewResolvingSender(resolve func(context.Context) (SMTPConfig, error)) *ResolvingSender {
	return &ResolvingSender{resolve: resolve}
}

func (s *ResolvingSender) Send(ctx context.Context, recipient, subject, body string) error {
	cfg, err := s.resolve(ctx)
	if err != nil {
		return err
	}
	return NewSMTPSender(cfg).Send(ctx, recipient, subject, body)
}

// ResolvingSlack loads the bot token at send time.
type ResolvingSlack struct {
	resolve func(context.Context) (string, error)
}

func NewResolvingSlack(resolve func(context.Context) (string, error)) *ResolvingSlack {
	return &ResolvingSlack{resolve: resolve}
}

func (s *ResolvingSlack) PostDM(ctx context.Context, userID, text string) error {
	token, err := s.resolve(ctx)
	if err != nil {
		return err
	}
	return NewSlackClient(token).PostDM(ctx, userID, text)
}
