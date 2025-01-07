package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/xarunoba/html2pdf-go"
)

func main() {
	// Create converter with custom options
	converter, err := html2pdf.New(
		html2pdf.WithWorkers(1),
		html2pdf.WithTimeout(30*time.Second),
		html2pdf.WithRetryAttempts(3),
		html2pdf.WithRetryDelay(time.Second),
		html2pdf.WithDebug(true),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer converter.Close()

	html := `
	<!DOCTYPE html>
	<html>
		<body>
			<h1>Hello, World!</h1>
		</body>
	</html>`

	// Convert with custom PDF options
	ctx := context.Background()
	pdf, err := converter.Convert(ctx, html,
		html2pdf.WithPaperSize(11, 8.5),
		html2pdf.WithMargins(0.25, 0.25, 0.25, 0.25),
		html2pdf.WithWaitForLoad(true),
		html2pdf.WithWaitForAnimations(500*time.Millisecond),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Save the PDF
	if err := os.WriteFile("output.pdf", pdf, 0644); err != nil {
		log.Fatal(err)
	}
}
