package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chromedp/cdproto/network" // REQUIRED for cookie extraction
	"github.com/chromedp/chromedp"
)

func main() {
	// --- 1. Credential Setup (Variable names maintained) ---
	username := os.Getenv("DUO_USER")
	password := os.Getenv("DUO_PASS")

	if username == "" || password == "" {
		log.Fatal("DUO_USER or DUO_PASS not set")
	}

	// --- 2. Browser Context Setup ---
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", false),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Enable the Network protocol for cookie access
	if err := chromedp.Run(ctx, network.Enable()); err != nil {
		log.Fatal(err)
	}

	var jwtToken string
	var pageTitle string

	// --- 3. Automation and Extraction ---
	err := chromedp.Run(ctx,
		// 1. Go to login page
		chromedp.Navigate("https://www.duolingo.com/log-in"),
		chromedp.WaitVisible(`input[placeholder="Email or username"]`, chromedp.ByQuery),

		// 2. Fill username and password
		chromedp.SendKeys(`input[placeholder="Email or username"]`, username, chromedp.ByQuery),
		chromedp.SendKeys(`input[placeholder="Password"]`, password, chromedp.ByQuery),

		// 3. Click login button and wait for navigation
		chromedp.Click(`button[type="submit"]`, chromedp.ByQuery),

		// 4. Wait for successful login (URL should change away from log-in page)
		chromedp.ActionFunc(func(ctx context.Context) error {
			for i := 0; i < 30; i++ {
				var currentURL string
				if err := chromedp.Location(&currentURL).Do(ctx); err != nil {
					return err
				}
				if currentURL != "" && !chromedp.Evaluate(`window.location.href.includes('log-in')`, nil).Do(ctx) {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
			return nil
		}),

		// 5. Get the page title
		chromedp.Title(&pageTitle),
	)

	if err != nil {
		log.Fatal(err)
	}

	// --- 4. Output ---
	fmt.Println("-------------------------------------------------")
	fmt.Printf("📄 Page title after login: %s\n", pageTitle)
	fmt.Println("-------------------------------------------------")

	// Output the extracted JWT token
	if jwtToken == "" {
		log.Fatal("Token not found (jwt_token cookie was missing)")
	}

	fmt.Println("-------------------------------------------------")
	fmt.Println("✅ Auth token extracted successfully:")
	fmt.Println(jwtToken)
	fmt.Println("-------------------------------------------------")
}
