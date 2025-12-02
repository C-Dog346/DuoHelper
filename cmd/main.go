// TO DO:
// - Improve error handling and logging
// - Add unit tests for functions
// - Modularise code for use on any windows computer with chrome
// - Allow user to specify Duolingo username
// - Improve data security (encrypt JWT token, dont store it on gihub)
// - Add support for other OS (macOS, Linux)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/go-toast/toast"
)

type Tokens struct {
	JWT string `json:"jwt"`
}

// loadToken reads the JWT token from tokens.json
func loadToken() string {
	exe, _ := os.Executable()
	tokenPath := filepath.Join(filepath.Dir(exe), "tokens.json")

	b, err := os.ReadFile(tokenPath)
	if err != nil {
		b, err = os.ReadFile("tokens.json")
		if err != nil {
			log.Fatalf("Failed to read tokens.json: %v", err)
		}
	}

	var t Tokens
	if err := json.Unmarshal(b, &t); err != nil {
		log.Fatalf("Invalid JSON: %v", err)
	}
	if t.JWT == "" {
		log.Fatal("JWT token is empty")
	}

	return t.JWT
}

// getUserInfo fetches user information from Duolingo API
func getUserInfo(jwt string) map[string]interface{} {
	req, _ := http.NewRequest("GET", "https://www.duolingo.com/users/CallumClow5", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&data)
	return data
}

// checkJWTValidity checks if the JWT token is valid based on user data
func checkJWTValidity(data map[string]interface{}) bool {
	if _, ok := data["username"]; !ok {
		return false
	}
	return true
}

func promptLogin() {
	fmt.Println("\n=== JWT Token Update Required ===")
	fmt.Println("Opening browser for login...")
	fmt.Println("Please log in to Duolingo. The window will close automatically after 3 minutes.")

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-gpu", false),
		chromedp.WindowSize(1280, 800),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	var jwt string

	err := chromedp.Run(ctx,
		chromedp.Navigate("https://www.duolingo.com"),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Poll for jwt_token cookie until found or timeout
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
					cookies, err := network.GetCookies().Do(ctx)
					if err != nil {
						continue
					}
					for _, cookie := range cookies {
						if cookie.Name == "jwt_token" {
							jwt = cookie.Value
							return nil
						}
					}
				}
			}
		}),
	)

	if err != nil || jwt == "" {
		fmt.Println("❌ Failed to extract JWT token.")
		fmt.Println("Please update tokens.json manually.")
		return
	}

	fmt.Printf("Extracted JWT: %s\n", jwt)

	tokens := Tokens{JWT: jwt}
	data, _ := json.MarshalIndent(tokens, "", "  ")

	// Save to bin directory
	binPath := filepath.Join("D:\\Code\\DuoHelper\\bin", "tokens.json")
	fmt.Printf("Saving to bin: %s\n", binPath)
	err = os.WriteFile(binPath, data, 0644)
	if err != nil {
		fmt.Printf("Failed to write to bin: %v\n", err)
	} else {
		fmt.Printf("✓ Saved to bin\n")
	}

	// Save to cmd directory
	cmdPath := filepath.Join("D:\\Code\\DuoHelper\\cmd", "tokens.json")
	fmt.Printf("Saving to cmd: %s\n", cmdPath)
	err = os.WriteFile(cmdPath, data, 0644)
	if err != nil {
		fmt.Printf("Failed to write to cmd: %v\n", err)
	} else {
		fmt.Printf("✓ Saved to cmd\n")
	}

	fmt.Println("\n✓ JWT token saved successfully!")
	fmt.Println("Please run the program again.")
}

// checkTodayTask checks if the user has completed today's task
func checkTodayTask(data map[string]interface{}) bool {
	extended, _ := data["streak_extended_today"].(bool)
	return extended
}

// sendNotification sends a desktop notification with the given message
func sendNotification(message string) {
	notification := toast.Notification{
		AppID:   "Duolingo Notifier",
		Title:   "Duolingo Reminder",
		Message: message,
	}
	if err := notification.Push(); err != nil {
		exec.Command("msg", "*", message).Run()
	}
}

// taskExists checks if the scheduled task already exists
func taskExists() bool {
	cmd := exec.Command("schtasks", "/Query", "/TN", "DuolingoDailyReminder")
	return cmd.Run() == nil
}

// createScheduledTask creates or updates the scheduled task
func createScheduledTask(exePath, time string) error {
	cmd := exec.Command("schtasks",
		"/Create", "/SC", "DAILY", "/TN", "DuolingoDailyReminder",
		"/TR", fmt.Sprintf(`"%s"`, exePath), "/ST", time, "/F", "/RL", "HIGHEST")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(out))
	}
	return nil
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "settime":
			if len(os.Args) < 3 {
				fmt.Println("Usage: DuoHelper.exe settime HH:MM")
				return
			}
			exe, _ := os.Executable()
			if err := createScheduledTask(exe, os.Args[2]); err != nil {
				log.Fatalf("Failed to update task: %v", err)
			}
			fmt.Printf("✓ Time updated to %s\n", os.Args[2])
			return

		case "help":
			fmt.Println("Commands:\n  settime HH:MM\n  help")
			return

		default:
			fmt.Printf("Unknown command. Run with 'help' for usage.\n")
			return
		}
	}

	if !taskExists() {
		fmt.Print("Enter daily reminder time (HH:MM, 24 hour format): ")
		var t string
		fmt.Scanln(&t)
		exe, _ := os.Executable()
		if err := createScheduledTask(exe, t); err != nil {
			log.Fatalf("Failed to create task: %v", err)
		}
		fmt.Println("✓ Reminder scheduled!")
		return
	}

	jwt := loadToken()
	data := getUserInfo(jwt)
	if !checkJWTValidity(data) {
		fmt.Println("❌ Invalid JWT token! Please update tokens.json.")
		sendNotification("Your Duolingo JWT token is invalid! ❌")
		promptLogin()
		return
	}
	doneToday := checkTodayTask(data)

	if doneToday {
		fmt.Println("✔ You have already done your Duolingo task today!")
		sendNotification("You have already completed today's Duolingo lesson! ✅")
	} else {
		fmt.Println("❌ You have NOT done your Duolingo task today!")
		sendNotification("You have NOT completed today's Duolingo lesson! ❌")
	}
}
