// rmq-go declares a RabbitMQ queue (quorum or classic, chosen in .env), publishes JSON
// messages to it at a fixed rate and consumes them back, logging every SENT and RECV.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	mrand "math/rand/v2"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	amqp "github.com/rabbitmq/amqp091-go"
)

type config struct{ host, port, user, pass, vhost, queue, qtype string }

// payload is the JSON body of every message.
type payload struct {
	Timestamp time.Time `json:"timestamp"`
	Random    string    `json:"random"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds) // slog's default handler writes through log
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// maxReadDelay caps -read-delay. With a prefetch of 100, a delivery can wait behind 99
// delayed ones, and RabbitMQ closes a channel whose delivery stays unacked for 30 minutes;
// at a 5 s average delay that wait is about 8 minutes, well clear of the limit.
const maxReadDelay = 10 * time.Second

func run() error {
	rate := flag.Int("rate", 1, "messages to send per second (1-1000)")
	readDelay := flag.Duration("read-delay", 0, fmt.Sprintf("wait a random time up to this long before reading each message, e.g. 500ms (max %s, 0 = off)", maxReadDelay))
	flag.Parse()
	// ponytail: one message per ticker tick; timers resolve ~1 ms, so 1000/s is the
	// ceiling. Batch several messages per tick to go higher.
	if *rate < 1 || *rate > 1000 {
		return fmt.Errorf("-rate must be between 1 and 1000, got %d", *rate)
	}
	if *readDelay < 0 || *readDelay > maxReadDelay {
		return fmt.Errorf("-read-delay must be between 0 and %s, got %s", maxReadDelay, *readDelay)
	}

	// A missing .env is fine as long as the settings are in the environment.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}
	c, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Credentials and vhost go in the config, not the URL, so no escaping is needed.
	addr := net.JoinHostPort(c.host, c.port)
	conn, err := amqp.DialConfig("amqp://"+addr, amqp.Config{
		SASL:  []amqp.Authentication{&amqp.PlainAuth{Username: c.user, Password: c.pass}},
		Vhost: c.vhost,
	})
	if err != nil {
		return fmt.Errorf("connect to %s: %w", addr, err)
	}
	defer conn.Close()
	slog.Info("connected", "host", addr, "vhost", c.vhost)

	// Separate channels for publishing and consuming.
	pub, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open publish channel: %w", err)
	}
	sub, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open consume channel: %w", err)
	}

	// Durable, non-exclusive and non-auto-delete suits both types (quorum queues require it).
	// x-queue-type is always sent, even for classic, because a vhost can default to another type.
	if _, err := sub.QueueDeclare(c.queue, true, false, false, false, amqp.Table{"x-queue-type": c.qtype}); err != nil {
		return fmt.Errorf("declare queue %q: %w", c.queue, err)
	}
	slog.Info("queue ready", "name", c.queue, "type", c.qtype)

	// Bound the client-side buffer so a slow terminal can't pile up deliveries.
	if err := sub.Qos(100, 0, false); err != nil {
		return fmt.Errorf("set prefetch: %w", err)
	}
	msgs, err := sub.Consume(c.queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %q: %w", c.queue, err)
	}
	if *readDelay > 0 {
		slog.Info("random read delay on", "max", *readDelay)
	}
	consumed := make(chan error, 1)
	go func() { consumed <- consume(msgs, *readDelay) }()

	slog.Info("producing; press Ctrl+C to stop", "rate", fmt.Sprintf("%d/s", *rate))
	tick := time.NewTicker(time.Second / time.Duration(*rate))
	defer tick.Stop()
	for {
		// Once Ctrl+C cancels ctx, publish fails with ctx.Err(); that is a clean stop.
		if err := publish(ctx, pub, c.queue); err != nil && ctx.Err() == nil {
			return fmt.Errorf("publish: %w", err)
		}
		select {
		case <-ctx.Done():
			slog.Info("stopping")
			return nil
		case err := <-consumed:
			return fmt.Errorf("consume: %w", err)
		case <-tick.C:
		}
	}
}

func loadConfig() (config, error) {
	var missing []string
	get := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}
	c := config{
		host:  get("RABBITMQ_HOST"),
		port:  get("RABBITMQ_PORT"),
		user:  get("RABBITMQ_USER"),
		pass:  get("RABBITMQ_PASS"),
		vhost: get("RABBITMQ_VHOST"),
		queue: get("RABBITMQ_QUEUE"),
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("missing settings (set them in .env or the environment): %s", strings.Join(missing, ", "))
	}

	// RABBITMQ_QUEUE_TYPE is optional: unset or empty means quorum.
	raw := os.Getenv("RABBITMQ_QUEUE_TYPE")
	c.qtype = strings.ToLower(strings.TrimSpace(raw))
	switch c.qtype {
	case "":
		c.qtype = "quorum"
	case "classic", "quorum":
	default:
		return c, fmt.Errorf("RABBITMQ_QUEUE_TYPE must be 'classic' or 'quorum', got '%s'", raw)
	}
	return c, nil
}

// publish sends one JSON message through the default exchange to queue.
func publish(ctx context.Context, ch *amqp.Channel, queue string) error {
	p := payload{Timestamp: time.Now(), Random: rand.Text()}
	body, _ := json.Marshal(p) // cannot fail: a time and a string
	// ponytail: no publisher confirms, so SENT means "written to the connection",
	// not "acknowledged by the broker". Add ch.Confirm + deferred confirms if it matters.
	err := ch.PublishWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
	if err != nil {
		return err
	}
	slog.Info("SENT", "random", p.Random, "ts", p.Timestamp)
	return nil
}

// consume logs and acks deliveries until the delivery channel closes. With maxDelay > 0
// each message first waits a random time in [0, maxDelay) and the RECV line shows it.
func consume(msgs <-chan amqp.Delivery, maxDelay time.Duration) error {
	for d := range msgs {
		var delay []any // becomes delay=<wait> on the log line when the option is on
		if maxDelay > 0 {
			// ponytail: messages are read one at a time, so delays add up and the consumer
			// manages at most 1/average-delay per second. Handle each delivery in its own
			// goroutine to overlap them.
			wait := mrand.N(maxDelay)
			time.Sleep(wait)
			delay = []any{"delay", wait.Round(time.Millisecond)}
		}
		var p payload
		if err := json.Unmarshal(d.Body, &p); err != nil {
			slog.Warn("RECV", append([]any{"err", err, "body", string(d.Body)}, delay...)...)
		} else {
			slog.Info("RECV", append([]any{"random", p.Random, "ts", p.Timestamp}, delay...)...)
		}
		if err := d.Ack(false); err != nil {
			return fmt.Errorf("ack: %w", err)
		}
	}
	return errors.New("delivery channel closed")
}
