package main

import (
	"log"
	"time"
	"trackr/internal/workers"
)

func main() {
	log.Println("Starting Trackr Background Workers...")

	// Start daily stats aggregator
	go runDailyStatsWorker()

	// Start webhook retry worker
	go runWebhookRetryWorker()

	// Start link expiry worker
	go runLinkExpiryWorker()

	// Keep process alive
	select {}
}

func runDailyStatsWorker() {
	// Run at 01:00 UTC daily
	for {
		now := time.Now().UTC()
		// Calculate time until next run (stub: 1 hour for demo, or real calc)
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 1, 0, 0, 0, time.UTC)
		duration := next.Sub(now)

		if duration < 0 {
			duration = time.Minute
		}

		log.Printf("Daily stats worker sleeping for %v", duration)
		time.Sleep(duration)

		log.Println("Running daily stats aggregation...")
		if err := workers.AggregateDailyStats(); err != nil {
			log.Printf("Error aggregating stats: %v", err)
		}
	}
}

func runWebhookRetryWorker() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		workers.RetryFailedWebhooks()
	}
}

func runLinkExpiryWorker() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		workers.ExpireLinks()
	}
}
