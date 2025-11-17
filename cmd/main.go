package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/chromedp/chromedp"
)

func main() {
    // Create a chromedp context
    ctx, cancel := chromedp.NewContext(context.Background())
    defer cancel()

    // Add a timeout
    ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    var pageTitle string

    err := chromedp.Run(ctx,
        chromedp.Navigate("https://www.duolingo.com/log-in"),
        chromedp.Title(&pageTitle),
    )

    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Page title is:", pageTitle)
}
