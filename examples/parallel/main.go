package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/xarunoba/html2pdf-go"
)

func main() {
	// Create a converter with custom options
	converter, err := html2pdf.New(
		html2pdf.WithWorkers(4),
		html2pdf.WithTimeout(30*time.Second),
		html2pdf.WithDebug(true),
		html2pdf.WithRetryAttempts(3),
		html2pdf.WithRetryDelay(time.Second),
	)
	if err != nil {
		log.Fatalf("Failed to create converter: %v", err)
	}
	defer converter.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var wg sync.WaitGroup
	errChan := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			html := fmt.Sprintf("<html><body><h1>Hello World %d</h1></body></html>", id)
			log.Printf("Converting document %d", id)

			pdf, err := converter.Convert(ctx, html,
				html2pdf.WithPaperSize(8.5, 11),
				html2pdf.WithMargins(0.5, 0.5, 0.5, 0.5),
				html2pdf.WithWaitForLoad(true),
				html2pdf.WithWaitForAnimations(500*time.Millisecond),
			)

			if err != nil {
				errChan <- fmt.Errorf("document %d conversion failed: %w", id, err)
				return
			}

			filename := fmt.Sprintf("output%d.pdf", id)
			if err := os.WriteFile(filename, pdf, 0644); err != nil {
				errChan <- fmt.Errorf("failed to write document %d: %w", id, err)
				return
			}

			log.Printf("Successfully converted document %d", id)
		}(i)
	}

	wg.Wait()
	close(errChan)

	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		log.Printf("Completed with %d errors:", len(errors))
		for _, err := range errors {
			log.Printf("- %v", err)
		}
		os.Exit(1)
	}

	log.Println("All conversions completed successfully")
}
