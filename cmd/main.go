// TO DO:
// Test window creds to see if the code works down all paths and edge cases
// - After the jwt is updated upon re-login, re-check if today's task is done
// - Improve error handling and logging
// - Add unit tests for functions
// - Modularise code for use on any windows computer with chrome
// - Allow user to specify Duolingo username
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
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/danieljoos/wincred"
	"github.com/go-toast/toast"
)

type Tokens struct {
	JWT string `json:"jwt"`
}

// loadToken reads the JWT token from Windows Credential Manager
func loadToken() string {
	cred, err := wincred.GetGenericCredential("DuoHelper_JWT")
	if err != nil {
		log.Fatalf("Failed to load JWT token from Credential Manager: %v", err)
	}

	if len(cred.CredentialBlob) == 0 {
		log.Fatal("JWT token is empty")
	}

	return string(cred.CredentialBlob)
}

// saveToken saves the JWT token to Windows Credential Manager
func saveToken(jwt string) bool {
	cred := wincred.NewGenericCredential("DuoHelper_JWT")
	cred.CredentialBlob = []byte(jwt)
	cred.Persist = wincred.PersistLocalMachine
	cred.UserName = "DuoHelper"

	err := cred.Write()
	if err != nil {
		fmt.Printf("Failed to save JWT token: %v\n", err)
		return false
	}
	return true
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
		fmt.Println("Please try logging in again.")
		return
	}

	fmt.Printf("✓ Extracted JWT token successfully\n")

	// Save to Windows Credential Manager
	if !saveToken(jwt) {
		fmt.Println("❌ Failed to save JWT token.")
		fmt.Println("Please contact support or try again.")
	}
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
		fmt.Println("❌ Invalid JWT token! Refreshing...")
		sendNotification("Your Duolingo JWT token is invalid! Refreshing...")
		promptLogin()

		// After successful login, reload token and check task
		jwt = loadToken()
		data = getUserInfo(jwt)
		if !checkJWTValidity(data) {
			fmt.Println("❌ Still unable to validate JWT after refresh. Please try again later.")
			return
		}
		fmt.Println("✓ JWT refreshed successfully! Checking today's task...")
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
