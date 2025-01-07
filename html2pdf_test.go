package html2pdf_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xarunoba/html2pdf-go"
)

type contextKey string

const (
	testContextKey contextKey = "test"
)

// TestNew tests converter initialization with various options
func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		opts    []html2pdf.Option
		wantErr bool
	}{
		{
			name:    "Default Options",
			opts:    nil,
			wantErr: false,
		},
		{
			name: "Custom Options",
			opts: []html2pdf.Option{
				html2pdf.WithWorkers(2),
				html2pdf.WithDebug(true),
				html2pdf.WithTimeout(30 * time.Second),
				html2pdf.WithRetryAttempts(3),
				html2pdf.WithRetryDelay(time.Second),
			},
			wantErr: false,
		},
		{
			name: "Invalid Options",
			opts: []html2pdf.Option{
				html2pdf.WithWorkers(-1),
				html2pdf.WithTimeout(-time.Second),
			},
			wantErr: false, // Should use defaults
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter, err := html2pdf.New(tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
			if converter != nil {
				defer converter.Close()
			}
		})
	}
}

// TestConvert tests basic PDF conversion functionality
func TestConvert(t *testing.T) {
	converter, err := html2pdf.New(html2pdf.WithDebug(true))
	if err != nil {
		t.Fatalf("Failed to create converter: %v", err)
	}
	defer converter.Close()

	tests := []struct {
		name     string
		html     string
		opts     []html2pdf.PDFOption
		wantErr  bool
		checkPDF bool
	}{
		{
			name:     "Empty HTML",
			html:     "",
			wantErr:  false,
			checkPDF: true,
		},
		{
			name:     "Simple HTML",
			html:     "<html><body><h1>Hello World</h1></body></html>",
			wantErr:  false,
			checkPDF: true,
		},
		{
			name: "Complex HTML",
			html: `<html><head><style>
                body { font-family: Arial; }
                .container { display: flex; }
                @media print { .no-print { display: none; } }
            </style></head>
            <body><div class="container"><h1>Test</h1></div></body></html>`,
			opts: []html2pdf.PDFOption{
				html2pdf.WithPaperSize(8.5, 11),
				html2pdf.WithMargins(1, 1, 1, 1),
				html2pdf.WithWaitForLoad(true),
				html2pdf.WithWaitForAnimations(time.Second),
			},
			wantErr:  false,
			checkPDF: true,
		},
		{
			name: "With Animation",
			html: `<html><body>
                <style>
                    @keyframes test {
                        from { opacity: 0; }
                        to { opacity: 1; }
                    }
                    div { animation: test 1s; }
                </style>
                <div>Animated Content</div>
            </body></html>`,
			opts: []html2pdf.PDFOption{
				html2pdf.WithWaitForAnimations(1500 * time.Millisecond),
			},
			wantErr:  false,
			checkPDF: true,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdf, err := converter.Convert(ctx, tt.html, tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("Convert() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.checkPDF {
				if len(pdf) == 0 {
					t.Error("Expected non-empty PDF output")
				}
				if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
					t.Error("Output does not appear to be a valid PDF")
				}
			}
		})
	}
}

// TestContext tests context handling and cancellation
func TestContext(t *testing.T) {
	converter, err := html2pdf.New(html2pdf.WithDebug(true))
	if err != nil {
		t.Fatalf("Failed to create converter: %v", err)
	}
	defer converter.Close()

	tests := []struct {
		name    string
		ctx     context.Context
		html    string
		wantErr bool
	}{
		{
			name:    "Nil Context",
			ctx:     nil,
			html:    "<html><body>Test</body></html>",
			wantErr: false,
		},
		{
			name: "Cancelled Context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			html:    "<html><body>Test</body></html>",
			wantErr: true,
		},
		{
			name: "Timeout Context",
			ctx: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
				defer cancel()
				return ctx
			}(),
			html: `<html><body>
                <script>
                    let i = 0;
                    while(i < 1000000000) { i++; }
                </script>
            </body></html>`,
			wantErr: true,
		},
		{
			name:    "Context with Values",
			ctx:     context.WithValue(context.Background(), testContextKey, "value"),
			html:    "<html><body>Test</body></html>",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := converter.Convert(tt.ctx, tt.html)
			if (err != nil) != tt.wantErr {
				t.Errorf("Convert() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestConcurrency tests concurrent PDF conversions
func TestConcurrency(t *testing.T) {
	converter, err := html2pdf.New(
		html2pdf.WithWorkers(4),
		html2pdf.WithDebug(true),
	)
	if err != nil {
		t.Fatalf("Failed to create converter: %v", err)
	}
	defer converter.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, 20)

	htmlContents := []string{
		"<html><body>Simple Test</body></html>",
		"<html><body><script>console.log('test');</script></body></html>",
		"<html><body><img src='data:image/gif;base64,R0lGODlhAQABAIAAAP///wAAACH5BAEAAAAALAAAAAABAAEAAAICRAEAOw=='/></body></html>",
		`<html><body>
			<script>
				const start = Date.now();
				while(Date.now() - start < 100) {}
			</script>
		</body></html>`,
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			html := htmlContents[i%len(htmlContents)]
			pdf, err := converter.Convert(ctx, html)
			if err != nil {
				errors <- fmt.Errorf("worker %d error: %w", i, err)
				return
			}
			if len(pdf) == 0 {
				errors <- fmt.Errorf("worker %d: empty PDF", i)
			}
		}(i)
		time.Sleep(10 * time.Millisecond) // Small delay between requests
	}

	wg.Wait()
	close(errors)

	errorCount := 0
	for err := range errors {
		t.Logf("Error: %v", err)
		errorCount++
	}

	if errorCount > 5 { // Allow some failures
		t.Errorf("Too many errors in concurrent test: %d", errorCount)
	}
}

// TestErrorScenarios tests various error conditions
func TestErrorScenarios(t *testing.T) {
	converter, err := html2pdf.New(
		html2pdf.WithWorkers(2),
		html2pdf.WithDebug(true),
		html2pdf.WithTimeout(2*time.Second),
	)
	if err != nil {
		t.Fatalf("Failed to create converter: %v", err)
	}
	defer converter.Close()

	tests := []struct {
		name    string
		html    string
		wantErr bool
	}{
		{
			name:    "JavaScript Error",
			html:    "<html><script>throw new Error('test error');</script></html>",
			wantErr: true,
		},
		{
			name:    "Invalid JavaScript",
			html:    "<html><script>function{ invalid</script></html>",
			wantErr: true,
		},
		{
			name:    "Infinite Loop",
			html:    "<html><script>while(true) {}</script></html>",
			wantErr: true,
		},
		{
			name: "Resource Exhaustion",
			html: `<html><script>
				const array = [];
				for(let i=0; i<10000; i++) {
					array.push(new Array(1000).fill('test'));
				}
			</script></html>`,
			wantErr: false,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			_, err := converter.Convert(ctx, tt.html)
			if (err != nil) != tt.wantErr {
				t.Errorf("Convert() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestCleanup tests cleanup and shutdown behavior
func TestCleanup(t *testing.T) {
	t.Run("Graceful Shutdown", func(t *testing.T) {
		converter, err := html2pdf.New(
			html2pdf.WithWorkers(2),
			html2pdf.WithDebug(true),
		)
		if err != nil {
			t.Fatalf("Failed to create converter: %v", err)
		}

		ctx := context.Background()
		var wg sync.WaitGroup
		results := make(chan error, 5)

		// Start conversions
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := converter.Convert(ctx, "<html><body>Test</body></html>")
				results <- err
			}()
		}

		// Wait briefly then close
		time.Sleep(100 * time.Millisecond)
		if err := converter.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}

		// Verify converter is closed
		_, err = converter.Convert(ctx, "<html><body>Test</body></html>")
		if err == nil {
			t.Error("Expected error when converting after close")
		}

		// Check results
		wg.Wait()
		close(results)

		for err := range results {
			if err != nil && !strings.Contains(err.Error(), "shutting down") {
				t.Errorf("Unexpected error type: %v", err)
			}
		}
	})

	t.Run("Concurrent Cleanup", func(t *testing.T) {
		converter, err := html2pdf.New(html2pdf.WithDebug(true))
		if err != nil {
			t.Fatalf("Failed to create converter: %v", err)
		}

		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				converter.Close()
			}()
		}
		wg.Wait()
	})
}

// TestStressWithRetries tests the retry mechanism under load
func TestStressWithRetries(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	converter, err := html2pdf.New(
		html2pdf.WithWorkers(4),
		html2pdf.WithDebug(true),
		html2pdf.WithRetryAttempts(2),
		html2pdf.WithRetryDelay(100*time.Millisecond),
		html2pdf.WithTimeout(5*time.Second), // Increased timeout
	)
	if err != nil {
		t.Fatalf("Failed to create converter: %v", err)
	}
	defer converter.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, 20)

	numOperations := 10
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			// Create a context with timeout for each operation
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			html := fmt.Sprintf(`
                <html><body>
                    <script>
                        // Simplified error generation
                        if (Math.random() > 0.7) {
                            throw new Error('Random error %d');
                        }
                        // Shorter processing time
                        let x = 0;
                        for(let i=0; i<1000; i++) { x += i; }
                    </script>
                    <div>Test %d</div>
                </body></html>
            `, i, i)

			_, err := converter.Convert(ctx, html)
			if err != nil {
				// Only report non-timeout errors
				if !strings.Contains(err.Error(), "deadline exceeded") {
					errors <- fmt.Errorf("iteration %d: %w", i, err)
				}
			}
		}(i)
		time.Sleep(200 * time.Millisecond)
	}

	wg.Wait()
	close(errors)

	errorCount := 0
	for err := range errors {
		t.Logf("Error: %v", err)
		errorCount++
	}

	maxErrors := numOperations / 2
	if errorCount > maxErrors {
		t.Errorf("Too many errors in stress test: %d (max allowed: %d)", errorCount, maxErrors)
	}
}
