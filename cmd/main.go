// TO DO:
// - Improve error handling and logging
// - Add unit tests for functions
// - Modularise code for use on any windows computer with chrome
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

const (
	credentialJWT      = "DuoHelper_JWT"
	credentialUsername = "DuoHelper_Username"
	credentialUser     = "DuoHelper"
	taskName           = "DuolingoDailyReminder"
	duolingoAPIBase    = "https://www.duolingo.com"
	loginTimeout       = 3 * time.Minute
	pollInterval       = 2 * time.Second
)

// loadToken reads the JWT token from Windows Credential Manager
func loadToken() (string, bool) {
	cred, err := wincred.GetGenericCredential(credentialJWT)
	if err != nil || len(cred.CredentialBlob) == 0 {
		return "", false
	}
	return string(cred.CredentialBlob), true
}

// loadUsername reads the Duolingo username from Windows Credential Manager
func loadUsername() (string, bool) {
	cred, err := wincred.GetGenericCredential(credentialUsername)
	if err != nil || len(cred.CredentialBlob) == 0 {
		return "", false
	}
	return string(cred.CredentialBlob), true
}

// saveUsername saves the Duolingo username to Windows Credential Manager
func saveUsername(username string) error {
	cred := wincred.NewGenericCredential(credentialUsername)
	cred.CredentialBlob = []byte(username)
	cred.Persist = wincred.PersistLocalMachine
	cred.UserName = credentialUser
	return cred.Write()
}

// saveToken saves the JWT token to Windows Credential Manager
func saveToken(jwt string) error {
	cred := wincred.NewGenericCredential(credentialJWT)
	cred.CredentialBlob = []byte(jwt)
	cred.Persist = wincred.PersistLocalMachine
	cred.UserName = credentialUser
	return cred.Write()
}

// getUserInfo fetches user information from Duolingo API
func getUserInfo(jwt, username string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", duolingoAPIBase+"/users/"+username, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return data, nil
}

// checkJWTValidity checks if the JWT token is valid based on user data
func checkJWTValidity(data map[string]interface{}) bool {
	_, ok := data["username"]
	return ok
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

	ctx, cancel = context.WithTimeout(ctx, loginTimeout)
	defer cancel()

	var jwt string

	err := chromedp.Run(ctx,
		chromedp.Navigate(duolingoAPIBase),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			ticker := time.NewTicker(pollInterval)
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

	fmt.Println("✓ Extracted JWT token successfully")

	if err := saveToken(jwt); err != nil {
		fmt.Printf("❌ Failed to save JWT token: %v\n", err)
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
	cmd := exec.Command("schtasks", "/Query", "/TN", taskName)
	return cmd.Run() == nil
}

// createScheduledTask creates or updates the scheduled task
func createScheduledTask(exePath, scheduleTime string) error {
	cmd := exec.Command("schtasks",
		"/Create", "/SC", "DAILY", "/TN", taskName,
		"/TR", fmt.Sprintf(`"%s"`, exePath), "/ST", scheduleTime, "/F", "/RL", "HIGHEST")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(out))
	}
	return nil
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "setup":
			if len(os.Args) < 4 {
				fmt.Println("Usage: DuoHelper.exe setup <username> <time>")
				fmt.Println("Example: DuoHelper.exe setup YourUsername 09:00")
				return
			}

			username := os.Args[2]
			if err := saveUsername(username); err != nil {
				log.Fatalf("Failed to save username: %v", err)
			}
			fmt.Printf("✓ Username set to: %s\n", username)

			exe, err := os.Executable()
			if err != nil {
				log.Fatalf("Failed to get executable path: %v", err)
			}
			scheduleTime := os.Args[3]
			if err := createScheduledTask(exe, scheduleTime); err != nil {
				log.Fatalf("Failed to create task: %v", err)
			}
			fmt.Printf("✓ Daily reminder scheduled for %s\n", scheduleTime)

			// Prompt for login
			fmt.Println("\n✓ Setup complete! Now logging in to extract JWT token...")
			promptLogin()

			_, ok := loadToken()
			if !ok {
				fmt.Println("❌ Login failed. Please run: DuoHelper.exe login")
				return
			}

			fmt.Println("\n✅ All done! DuoHelper will check your Duolingo progress daily.")
			fmt.Println("Run 'DuoHelper.exe help' to see available commands.")
			return

		case "settime":
			if len(os.Args) < 3 {
				fmt.Println("Usage: DuoHelper.exe settime HH:MM")
				return
			}
			exe, err := os.Executable()
			if err != nil {
				log.Fatalf("Failed to get executable path: %v", err)
			}
			if err := createScheduledTask(exe, os.Args[2]); err != nil {
				log.Fatalf("Failed to update task: %v", err)
			}
			fmt.Printf("✓ Time updated to %s\n", os.Args[2])
			return

		case "setusername":
			if len(os.Args) < 3 {
				fmt.Println("Usage: DuoHelper.exe setusername <username>")
				return
			}
			if err := saveUsername(os.Args[2]); err != nil {
				fmt.Printf("❌ Failed to save username: %v\n", err)
				return
			}
			fmt.Printf("✓ Username set to: %s\n", os.Args[2])
			return

		case "login":
			fmt.Println("Opening browser for login...")
			promptLogin()
			_, ok := loadToken()
			if ok {
				fmt.Println("✓ Login successful!")
			} else {
				fmt.Println("❌ Login failed. Please try again.")
			}
			return

		case "help":
			fmt.Println("DuoHelper - Duolingo Daily Reminder")
			fmt.Println("\nSetup (first time):")
			fmt.Println("  setup <username> <time>    Set up username and daily reminder")
			fmt.Println("                             Example: DuoHelper.exe setup YourUsername 09:00")
			fmt.Println("\nCommands:")
			fmt.Println("  settime HH:MM              Change reminder time")
			fmt.Println("  setusername <username>     Change Duolingo username")
			fmt.Println("  login                      Re-login to refresh JWT token")
			fmt.Println("  help                       Show this help message")
			return

		default:
			fmt.Printf("Unknown command '%s'. Run 'DuoHelper.exe help' for usage.\n", os.Args[1])
			return
		}
	}

	// Silent run mode (for scheduled task)
	username, ok := loadUsername()
	if !ok {
		sendNotification("DuoHelper setup incomplete. Please run: DuoHelper.exe setup <username> <time>")
		return
	}

	if !taskExists() {
		sendNotification("DuoHelper scheduled task not found. Please run setup again.")
		return
	}

	jwt, ok := loadToken()
	if !ok {
		sendNotification("DuoHelper: JWT token missing. Please run: DuoHelper.exe login")
		return
	}

	data, err := getUserInfo(jwt, username)
	if err != nil || !checkJWTValidity(data) {
		sendNotification("DuoHelper: Invalid JWT token. Please run: DuoHelper.exe login")
		return
	}

	if checkTodayTask(data) {
		sendNotification("You have already completed today's Duolingo lesson! ✅")
	} else {
		sendNotification("You have NOT completed today's Duolingo lesson! ❌")
	}
}
