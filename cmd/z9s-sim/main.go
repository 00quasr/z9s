// z9s-sim drives the examples/order-fulfillment.bpmn process with
// realistic traffic: it starts instances at a configurable rate and works
// the jobs with latency, transient failures, declined payments (BPMN
// errors), and the occasional exhausted-retries incident — so a local
// cluster looks and behaves like a busy one.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/00quasr/z9s/internal/camunda"
	"github.com/00quasr/z9s/internal/config"
)

const processID = "order-fulfillment"

var customers = []string{"acme-gmbh", "globex", "initech", "umbrella-ag", "wayne-ltd", "stark-industries"}
var regions = []string{"eu-central", "eu-west", "us-east", "apac"}

func main() {
	addr := flag.String("addr", "", "cluster base URL (same semantics as z9s)")
	profile := flag.String("profile", "", "c8ctl profile to connect with")
	rate := flag.Float64("rate", 8, "instances started per minute")
	burst := flag.Int("burst", 6, "instances to start immediately on launch")
	flag.Parse()

	prof, warnings, err := config.Resolve(*profile, *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "z9s-sim:", err)
		os.Exit(1)
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "z9s-sim:", w)
	}
	var rt http.RoundTripper
	switch prof.Auth {
	case config.AuthBasic:
		rt = camunda.BasicAuthTransport(prof.Username, prof.Password)
	case config.AuthOAuth:
		rt = camunda.OAuthTransport(prof.OAuthURL, prof.ClientID, prof.ClientSecret, prof.Audience, prof.Scope)
	}
	client := camunda.NewClient(prof.BaseURL, rt)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("z9s-sim: %s (%s) · %.1f instances/min · ctrl+c to stop\n", prof.BaseURL, prof.Label(), *rate)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		startInstances(ctx, client, *rate, *burst)
	}()
	for jobType, handle := range handlers {
		wg.Add(1)
		go func(jobType string, handle handler) {
			defer wg.Done()
			workJobs(ctx, client, jobType, handle)
		}(jobType, handle)
	}
	wg.Wait()
	fmt.Println("\nz9s-sim: stopped")
}

var orderSeq atomic.Int64

func startInstances(ctx context.Context, client *camunda.Client, perMinute float64, burst int) {
	start := func() {
		n := orderSeq.Add(1)
		vars := map[string]any{
			"orderId":  fmt.Sprintf("ORD-%04d", 1000+n),
			"amount":   10 + rand.Intn(1990),
			"customer": customers[rand.Intn(len(customers))],
			"region":   regions[rand.Intn(len(regions))],
			"items":    1 + rand.Intn(8),
		}
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		key, err := client.CreateProcessInstanceByID(cctx, processID, vars)
		if err != nil {
			fmt.Println("start failed:", err)
			return
		}
		fmt.Printf("▶ %s started (instance %s)\n", vars["orderId"], key)
	}

	for range burst {
		if ctx.Err() != nil {
			return
		}
		start()
	}
	interval := time.Duration(float64(time.Minute) / perMinute)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			start()
		}
	}
}

// handler decides a job's fate and returns a log line.
type handler func(ctx context.Context, client *camunda.Client, job camunda.Job) (string, error)

var handlers = map[string]handler{
	"z9s-sim-validate": func(ctx context.Context, c *camunda.Client, j camunda.Job) (string, error) {
		valid := rand.Float64() < 0.90
		if err := c.CompleteJob(ctx, j.JobKey, map[string]any{"valid": valid}); err != nil {
			return "", err
		}
		if !valid {
			return fmt.Sprintf("✗ %s rejected by validation", orderOf(j)), nil
		}
		return fmt.Sprintf("✓ %s validated", orderOf(j)), nil
	},
	"z9s-sim-inventory": func(ctx context.Context, c *camunda.Client, j camunda.Job) (string, error) {
		if rand.Float64() < 0.04 {
			if err := c.FailJob(ctx, j.JobKey, j.Retries-1, "Warehouse service unavailable (transient)"); err != nil {
				return "", err
			}
			return fmt.Sprintf("⚠ %s inventory hiccup (%d retries left)", orderOf(j), j.Retries-1), nil
		}
		if err := c.CompleteJob(ctx, j.JobKey, map[string]any{"reserved": true}); err != nil {
			return "", err
		}
		return fmt.Sprintf("✓ %s inventory reserved", orderOf(j)), nil
	},
	"z9s-sim-payment": func(ctx context.Context, c *camunda.Client, j camunda.Job) (string, error) {
		switch r := rand.Float64(); {
		case r < 0.10:
			if err := c.ThrowJobError(ctx, j.JobKey, "PAYMENT_DECLINED", "Card declined by issuer"); err != nil {
				return "", err
			}
			return fmt.Sprintf("⚡ %s payment declined", orderOf(j)), nil
		case r < 0.20:
			if err := c.FailJob(ctx, j.JobKey, j.Retries-1, "Payment gateway timeout after 30s"); err != nil {
				return "", err
			}
			return fmt.Sprintf("⚠ %s payment timeout (%d retries left)", orderOf(j), j.Retries-1), nil
		default:
			vars := map[string]any{"paymentId": fmt.Sprintf("PAY-%06d", rand.Intn(1000000))}
			if err := c.CompleteJob(ctx, j.JobKey, vars); err != nil {
				return "", err
			}
			return fmt.Sprintf("✓ %s payment captured", orderOf(j)), nil
		}
	},
	"z9s-sim-ship": func(ctx context.Context, c *camunda.Client, j camunda.Job) (string, error) {
		vars := map[string]any{"trackingId": fmt.Sprintf("TRK-%08d", rand.Intn(100000000))}
		if err := c.CompleteJob(ctx, j.JobKey, vars); err != nil {
			return "", err
		}
		return fmt.Sprintf("✓ %s shipped", orderOf(j)), nil
	},
	"z9s-sim-notify": func(ctx context.Context, c *camunda.Client, j camunda.Job) (string, error) {
		if err := c.CompleteJob(ctx, j.JobKey, nil); err != nil {
			return "", err
		}
		return fmt.Sprintf("✓ %s customer notified of cancellation", orderOf(j)), nil
	},
}

func workJobs(ctx context.Context, client *camunda.Client, jobType string, handle handler) {
	for ctx.Err() == nil {
		actx, cancel := context.WithTimeout(ctx, 12*time.Second)
		jobs, err := client.ActivateJobs(actx, jobType, "z9s-sim", 8, 60*time.Second)
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				fmt.Printf("%s: activation error: %v\n", jobType, err)
				sleep(ctx, 3*time.Second)
			}
			continue
		}
		for _, job := range jobs {
			// Simulated work latency keeps tokens visibly in flight.
			sleep(ctx, time.Duration(200+rand.Intn(1300))*time.Millisecond)
			if ctx.Err() != nil {
				return
			}
			hctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			line, err := handle(hctx, client, job)
			cancel()
			if err != nil {
				fmt.Printf("%s: %v\n", jobType, err)
				continue
			}
			fmt.Println(line)
		}
		if len(jobs) == 0 {
			sleep(ctx, 1500*time.Millisecond)
		}
	}
}

func orderOf(j camunda.Job) string {
	if id, ok := j.Variables["orderId"].(string); ok {
		return id
	}
	return j.ProcessInstanceKey
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
