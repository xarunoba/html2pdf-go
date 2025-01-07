package main

import (
	"context"
	"log"
	"os"

	"github.com/xarunoba/html2pdf-go"
)

func main() {
	// Create a new converter with default options
	converter, err := html2pdf.New()
	if err != nil {
		log.Fatal(err)
	}
	defer converter.Close()

	// HTML content to convert
	html := `
        <!DOCTYPE html>
        <html>
            <body>
                <h1>Hello, World!</h1>
            </body>
        </html>
    `

	// Convert HTML to PDF
	ctx := context.Background()
	pdf, err := converter.Convert(ctx, html)
	if err != nil {
		log.Fatal(err)
	}

	// Save the PDF
	if err := os.WriteFile("output.pdf", pdf, 0644); err != nil {
		log.Fatal(err)
	}
}
