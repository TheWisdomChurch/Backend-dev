package tasks

import (
    "fmt"
    "time"
    
    "wisdom-house-backend/internal/email"
)

type EmailTask struct {
    To      string
    Subject string
    Body    string
    Retries int
    sender  *email.Sender
}

func NewEmailTask(sender *email.Sender, to, subject, body string) *EmailTask {
    return &EmailTask{
        To:      to,
        Subject: subject,
        Body:    body,
        Retries: 3, // Default retry count
        sender:  sender,
    }
}

func (t *EmailTask) Execute() error {
    return t.sender.SendHTML(t.To, t.Subject, t.Body)
}

func (t *EmailTask) Name() string {
    return fmt.Sprintf("email_task_%s_%d", t.To, time.Now().Unix())
}

func (t *EmailTask) RetryCount() int {
    return t.Retries
}

// WelcomeEmailTask specific implementation
type WelcomeEmailTask struct {
    EmailTask
    Name string
}

func NewWelcomeEmailTask(sender *email.Sender, to, name string) *WelcomeEmailTask {
    subject := "Welcome to Wisdom House Church!"
    body := fmt.Sprintf(`
    <!DOCTYPE html>
    <html>
    <body style="font-family: Arial, sans-serif; line-height: 1.6;">
        <h2>Welcome to Wisdom House Church, %s!</h2>
        <p>We're excited to have you join our spiritual community.</p>
        <p>Stay connected for updates on sermons, events, and community activities.</p>
        <br>
        <p>Blessings,<br>The Wisdom House Team</p>
    </body>
    </html>`, name)
    
    return &WelcomeEmailTask{
        EmailTask: *NewEmailTask(sender, to, subject, body),
        Name:      name,
    }
}