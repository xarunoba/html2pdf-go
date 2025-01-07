// Package html2pdf provides functionality to convert HTML content to PDF documents using Chrome/Chromium.
// It manages a pool of Chrome instances to handle concurrent conversions efficiently and safely.
package html2pdf

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

var (
	ErrEmptyPDF   = errors.New("PDF generation produced empty output")
	ErrTimeout    = errors.New("conversion timeout exceeded")
	ErrInitialize = errors.New("failed to initialize converter")
	ErrConversion = errors.New("conversion failed")
	ErrCleanup    = errors.New("failed to cleanup resources")
	ErrShutdown   = errors.New("converter is shutting down")
)

type (
	// Option represents a function that modifies Converter configuration.
	Option func(*Converter)

	// PDFOption represents a function that modifies PDF generation settings.
	PDFOption func(*pdfConfig)

	// Converter manages a pool of Chrome instances for converting HTML to PDF.
	// It handles concurrent conversions and cleanup of resources.
	Converter struct {
		workers     []*worker
		workerPool  chan *worker
		cfg         config
		cleanupOnce sync.Once
		done        chan struct{}
		ctx         context.Context
		cancel      context.CancelFunc
	}

	config struct {
		numWorkers        int
		pdfDefaults       pdfConfig
		conversionTimeout time.Duration
		retryAttempts     int
		retryDelay        time.Duration
		debug             bool
	}

	pdfConfig struct {
		paperWidth        float64
		paperHeight       float64
		marginTop         float64
		marginRight       float64
		marginBottom      float64
		marginLeft        float64
		waitForLoad       bool
		waitForAnimations time.Duration
		waitTimeout       time.Duration
	}

	worker struct {
		ctx    context.Context
		cancel context.CancelFunc
		id     int
		isMain bool
	}
)

func defaultConfig() config {
	return config{
		numWorkers: 1,
		pdfDefaults: pdfConfig{
			paperWidth:        8.5,
			paperHeight:       11,
			marginTop:         0.5,
			marginRight:       0.5,
			marginBottom:      0.5,
			marginLeft:        0.5,
			waitForLoad:       true,
			waitForAnimations: 500 * time.Millisecond,
			waitTimeout:       60 * time.Second,
		},
		conversionTimeout: 60 * time.Second,
		retryAttempts:     3,
		retryDelay:        time.Second,
		debug:             false,
	}
}

// Converter options

// WithWorkers sets the number of concurrent Chrome workers.
// The number must be positive, otherwise it will be ignored.
// If n <= 0, the default number of workers (1) is used.
func WithWorkers(n int) Option {
	return func(c *Converter) {
		if n > 0 {
			c.cfg.numWorkers = n
		}
	}
}

// WithDebug enables or disables debug logging.
// The debug flag must be a boolean value, otherwise it will be false.
func WithDebug(debug bool) Option {
	return func(c *Converter) {
		c.cfg.debug = debug
	}
}

// WithTimeout sets the maximum duration for a single conversion operation.
// The duration must be positive, otherwise it will be ignored.
// If d <= 0, the default timeout (60s) is used.
func WithTimeout(d time.Duration) Option {
	return func(c *Converter) {
		if d > 0 {
			c.cfg.conversionTimeout = d
		}
	}
}

// WithRetryAttempts sets the number of retry attempts for failed conversions.
// The number must be positive, otherwise it will be ignored.
// If n <= 0, the default number of attempts (3) is used.
func WithRetryAttempts(n int) Option {
	return func(c *Converter) {
		if n > 0 {
			c.cfg.retryAttempts = n
		}
	}
}

// WithRetryDelay sets the delay between retry attempts in seconds.
// The duration must be positive, otherwise it will be ignored.
// If d <= 0, the default delay (1s) is used.
func WithRetryDelay(d time.Duration) Option {
	return func(c *Converter) {
		if d > 0 {
			c.cfg.retryDelay = d
		}
	}
}

// PDF options

// WithPaperSize sets the PDF paper size in inches. The width and height must be positive, otherwise they will be ignored.
// If width or height <= 0, the default size (8.5x11) is used.
func WithPaperSize(width, height float64) PDFOption {
	return func(c *pdfConfig) {
		if width > 0 {
			c.paperWidth = width
		}
		if height > 0 {
			c.paperHeight = height
		}
	}
}

// WithMargins sets the PDF margins in inches.
// The top, right, bottom, and left values must be positive, otherwise they will be ignored.
// If any value is <= 0, the default margins (0.5x0.5) are used.
func WithMargins(top, right, bottom, left float64) PDFOption {
	return func(c *pdfConfig) {
		if top > 0 {
			c.marginTop = top
		}
		if right > 0 {
			c.marginRight = right
		}
		if bottom > 0 {
			c.marginBottom = bottom
		}
		if left > 0 {
			c.marginLeft = left
		}
	}
}

// WithWaitForLoad sets whether to wait for the page to load before generating the PDF.
// The wait flag must be a boolean value, otherwise it will be false.
func WithWaitForLoad(wait bool) PDFOption {
	return func(c *pdfConfig) {
		c.waitForLoad = wait
	}
}

// WithWaitForAnimations sets how long to wait for animations to complete in milliseconds.
// The duration must be positive, otherwise it will be ignored.
// If d <= 0, the default duration (500ms) is used.
func WithWaitForAnimations(d time.Duration) PDFOption {
	return func(c *pdfConfig) {
		if d > 0 {
			c.waitForAnimations = d
		}
	}
}

// WithWaitTimeout sets the maximum duration to wait for page load and animations in seconds.
// The duration must be positive, otherwise it will be ignored.
// If d <= 0, the default timeout (60s) is used.
func WithWaitTimeout(d time.Duration) PDFOption {
	return func(c *pdfConfig) {
		if d > 0 {
			c.waitTimeout = d
		}
	}
}

// New creates a new Converter instance with the specified options.
// It initializes Chrome workers and sets up signal handling for graceful shutdown.
// Returns an error if initialization fails.
func New(opts ...Option) (*Converter, error) {
	// Create context with cancellation for the entire converter
	ctx, cancel := context.WithCancel(context.Background())

	c := &Converter{
		cfg:    defaultConfig(),
		done:   make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}

	for _, opt := range opts {
		opt(c)
	}

	if err := c.initialize(); err != nil {
		if cleanupErr := c.cleanup(); cleanupErr != nil {
			c.logDebug("cleanup error during initialization: %v", cleanupErr)
		}
		return nil, fmt.Errorf("%w: %v", ErrInitialize, err)
	}

	go c.handleSignals()

	return c, nil
}

func (c *Converter) handleSignals() {
	sigChan := make(chan os.Signal, 1)
	if runtime.GOOS == "windows" {
		signal.Notify(sigChan, os.Interrupt)
	} else {
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	}

	sig := <-sigChan
	c.logDebug("received signal: %v", sig)

	// Cancel the converter context to stop new operations
	c.cancel()

	if err := c.cleanup(); err != nil {
		c.logDebug("cleanup error during signal handling: %v", err)
	}

	if sig == syscall.SIGQUIT {
		return
	}

	close(c.done)
	os.Exit(0)
}

func (c *Converter) initialize() error {
	ctx := context.Background()
	mainContext, mainCancel := chromedp.NewContext(ctx)

	mainWorker := &worker{
		ctx:    mainContext,
		cancel: mainCancel,
		id:     0,
		isMain: true,
	}

	if err := chromedp.Run(mainWorker.ctx); err != nil {
		mainCancel()
		return fmt.Errorf("failed to initialize main worker: %w", err)
	}

	c.workers = []*worker{mainWorker}
	c.workerPool = make(chan *worker, c.cfg.numWorkers)
	c.workerPool <- mainWorker

	for i := 1; i < c.cfg.numWorkers; i++ {
		c.logDebug("initializing worker %d", i)

		tabContext, tabCancel := chromedp.NewContext(mainWorker.ctx)
		w := &worker{
			ctx:    tabContext,
			cancel: tabCancel,
			id:     i,
			isMain: false,
		}

		if err := chromedp.Run(w.ctx); err != nil {
			if cleanupErr := c.cleanup(); cleanupErr != nil {
				c.logDebug("cleanup error during initialization: %v", cleanupErr)
			}
			return fmt.Errorf("failed to initialize worker %d: %w", i, err)
		}

		c.workers = append(c.workers, w)
		c.workerPool <- w
	}

	c.logDebug("initialized %d workers", len(c.workers))
	return nil
}

func (c *Converter) cleanup() error {
	var err error
	c.cleanupOnce.Do(func() {
		err = c.performCleanup()
	})
	return err
}

func (c *Converter) performCleanup() error {
	if c.workers == nil {
		return nil
	}

	// Cancel the main context if not already cancelled
	c.cancel()

	var errs []error

	if c.workerPool != nil {
		// Safely drain the worker pool
		for {
			select {
			case <-c.workerPool:
			default:
				goto poolDrained
			}
		}
	poolDrained:
		close(c.workerPool)
	}

	// Cancel non-main workers first
	for _, w := range c.workers {
		if w != nil && !w.isMain {
			w.cancel()
			c.logDebug("worker %d shutdown", w.id)
		}
	}

	// Cancel main worker last
	for _, w := range c.workers {
		if w != nil && w.isMain {
			w.cancel()
			c.logDebug("main worker shutdown")
			break
		}
	}

	c.workers = nil
	c.workerPool = nil

	if len(errs) > 0 {
		return fmt.Errorf("%w: %v", ErrCleanup, errs)
	}
	return nil
}

// Convert transforms the provided HTML string into a PDF document.
// It uses a pooled Chrome instance and applies the specified PDF options.
// The context can be used to cancel the operation or set a deadline.
// Returns the PDF as a byte slice or an error if the conversion fails.
func (c *Converter) Convert(ctx context.Context, html string, opts ...PDFOption) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	pdfCfg := c.cfg.pdfDefaults
	for _, opt := range opts {
		opt(&pdfCfg)
	}

	ctx, cancel := context.WithTimeout(ctx, c.cfg.conversionTimeout)
	defer cancel()

	for attempt := 0; attempt < c.cfg.retryAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: %v", ErrTimeout, ctx.Err())
		default:
			pdf, err := c.tryConvert(ctx, html, &pdfCfg)
			if err == nil {
				return pdf, nil
			} else if errors.Is(ErrShutdown, err) {
				return nil, err
			}

			c.logDebug("attempt %d failed: %v", attempt+1, err)
			time.Sleep(c.cfg.retryDelay)
		}
	}

	return nil, fmt.Errorf("%w: exceeded retry attempts", ErrConversion)
}

func (c *Converter) tryConvert(ctx context.Context, html string, cfg *pdfConfig) ([]byte, error) {
	// Create a combined context that's cancelled if either the converter or operation context is cancelled
	combinedCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		select {
		case <-c.ctx.Done():
			cancel()
		case <-combinedCtx.Done():
		}
	}()

	select {
	case worker, ok := <-c.workerPool:
		if !ok {
			return nil, ErrShutdown
		}
		c.logDebug("using worker %d", worker.id)

		// Use defer with a closure to safely return the worker
		defer func() {
			select {
			case <-c.ctx.Done():
				// Don't return worker if shutting down
			default:
				c.workerPool <- worker
			}
		}()

		return c.processConversion(worker.ctx, html, cfg, combinedCtx)

	case <-combinedCtx.Done():
		if c.ctx.Err() != nil {
			return nil, ErrShutdown
		}
		return nil, ctx.Err()
	}
}

func (c *Converter) processConversion(workerCtx context.Context, html string, cfg *pdfConfig, ctx context.Context) ([]byte, error) {
	dataURL := "data:text/html;charset=utf-8;base64," + base64.StdEncoding.EncodeToString([]byte(html))

	jsErrorChan := make(chan error, 1)
	var pdf []byte

	chromedp.ListenTarget(workerCtx, func(ev interface{}) {
		if e, ok := ev.(*cdpruntime.EventExceptionThrown); ok {
			select {
			case jsErrorChan <- fmt.Errorf("JavaScript error: %s", e.ExceptionDetails.Error()):
			default:
			}
		}
	})

	actions := buildActions(dataURL, cfg, &pdf)

	done := make(chan error, 1)
	go func() {
		done <- chromedp.Run(workerCtx, actions...)
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case jsErr := <-jsErrorChan:
		return nil, jsErr
	case err := <-done:
		if err != nil {
			return nil, err
		}
		if len(pdf) == 0 {
			return nil, ErrEmptyPDF
		}
		return pdf, nil
	}
}

func buildActions(dataURL string, cfg *pdfConfig, pdf *[]byte) []chromedp.Action {
	actions := []chromedp.Action{
		cdpruntime.Enable(),
		chromedp.Navigate(dataURL),
	}

	if cfg.waitForLoad {
		actions = append(actions,
			network.Enable(),
			chromedp.ActionFunc(func(ctx context.Context) error {
				if err := network.SetExtraHTTPHeaders(network.Headers{}).Do(ctx); err != nil {
					return err
				}
				return network.SetCacheDisabled(true).Do(ctx)
			}),
			chromedp.WaitReady("body", chromedp.ByQuery),
		)
	}

	if cfg.waitForAnimations > 0 {
		actions = append(actions, chromedp.Sleep(cfg.waitForAnimations))
	}

	actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
		buf, _, err := page.PrintToPDF().
			WithPaperWidth(cfg.paperWidth).
			WithPaperHeight(cfg.paperHeight).
			WithMarginTop(cfg.marginTop).
			WithMarginRight(cfg.marginRight).
			WithMarginBottom(cfg.marginBottom).
			WithMarginLeft(cfg.marginLeft).
			WithPrintBackground(true).
			WithPreferCSSPageSize(true).
			Do(ctx)
		if err != nil {
			return fmt.Errorf("PDF generation failed: %w", err)
		}
		*pdf = buf
		return nil
	}))

	return actions
}

// Close shuts down the Converter and releases all associated resources.
// It ensures all Chrome instances are properly terminated.
// Should be called when the Converter is no longer needed.
func (c *Converter) Close() error {
	c.cancel() // Cancel the main context first
	err := c.cleanup()
	select {
	case <-c.done:
		// Cleanup already performed by signal handler
	default:
		close(c.done)
	}
	return err
}

func (c *Converter) logDebug(format string, args ...interface{}) {
	if c.cfg.debug {
		log.Printf("[html2pdf] "+format, args...)
	}
}
