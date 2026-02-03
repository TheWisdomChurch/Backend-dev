package email

import (
	"context"
	"log"
	"time"
)

type Job struct {
	To      string
	Subject string
	Body    string
}

type Queue struct {
	sender *Sender
	ch     chan Job
	logger *log.Logger
}

func NewQueue(sender *Sender, logger *log.Logger, buffer int) *Queue {
	if buffer <= 0 {
		buffer = 500
	}
	return &Queue{
		sender: sender,
		ch:     make(chan Job, buffer),
		logger: logger,
	}
}

func (q *Queue) Start(workers int) {
	if workers <= 0 {
		workers = 2
	}
	for i := 0; i < workers; i++ {
		go func(workerID int) {
			for job := range q.ch {
				ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
				err := q.sender.SendHTMLContext(ctx, job.To, job.Subject, job.Body)
				cancel()
				if err != nil && q.logger != nil {
					q.logger.Printf("email worker=%d send failed to=%s err=%v", workerID, job.To, err)
				}
			}
		}(i + 1)
	}
}

func (q *Queue) Enqueue(job Job) bool {
	select {
	case q.ch <- job:
		return true
	default:
		// queue full: drop or implement fallback
		if q.logger != nil {
			q.logger.Printf("email queue full, dropping to=%s subject=%s", job.To, job.Subject)
		}
		return false
	}
}

// SendHTML implements service.EmailSender by enqueueing the job for async send.
func (q *Queue) SendHTML(to, subject, body string) error {
	if q == nil {
		return nil
	}
	ok := q.Enqueue(Job{To: to, Subject: subject, Body: body})
	if ok {
		return nil
	}
	// If queue is full, fail silently to avoid blocking critical flows.
	return nil
}
