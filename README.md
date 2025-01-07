# html2pdf-go

A simple Go package for converting HTML content to PDF using the [`chromedp` package](https://github.com/chromedp/chromedp) under the hood. The package supports concurrent PDF generation through a worker pool system.

## Features

- Concurrent HTML to PDF conversion using Chrome/Chromium
- Configurable worker pool size
- Customizable PDF output settings (page size, margins, etc.)
- Support for waiting for page load and animations
- Automatic retry mechanism for failed conversions
- Debug logging option
- Resource cleanup with graceful shutdown

## Installation

```bash
go get github.com/xarunoba/html2pdf-go
```

Ensure you have Chrome or Chromium installed on your system.

## Usage

### Basic Usage

```go
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
```

For more usage examples, please check the [examples](/examples/) directory.

## Configuration Options

### Converter Options

| Option | Description | Default |
|--------|-------------|---------|
| WithWorkers | Number of concurrent Chrome tabs | 1 |
| WithTimeout | Maximum time for conversion | 60 seconds |
| WithRetryAttempts | Number of retry attempts | 3 |
| WithRetryDelay | Delay between retries | 1 second |
| WithDebug | Enable debug logging | false |

### PDF Options

| Option | Description | Default |
|--------|-------------|---------|
| WithPaperSize | Page width and height (inches) | 8.5 x 11 |
| WithMargins | Page margins (inches) | 0.5 all sides |
| WithWaitForLoad | Wait for page to load | true |
| WithWaitForAnimations | Animation completion wait time | 500ms |
| WithWaitTimeout | Maximum wait time | 60s |

## Considerations

- This package uses Chrome/Chromium for PDF generation, which provides excellent CSS support and rendering accuracy but may not be the fastest solution for high-volume processing.
- Memory usage scales with the number of workers, as each worker maintains a Chrome tab.
- Performance can vary based on system resources, document complexity, and whether the HTML contains external resources.
- For production environments, consider running benchmarks to determine optimal worker pool size for your use case.

## Requirements

- Go 1.23 or later
- Chrome or Chromium browser installed on the system
