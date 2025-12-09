// go build -o bin\DuoHelper.exe .\cmd\main.go
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

// Windows Credential Manager functions
func loadToken() (string, bool) {
	cred, err := wincred.GetGenericCredential(credentialJWT)
	if err != nil || len(cred.CredentialBlob) == 0 {
		return "", false
	}
	return string(cred.CredentialBlob), true
}

func saveToken(jwt string) error {
	cred := wincred.NewGenericCredential(credentialJWT)
	cred.CredentialBlob = []byte(jwt)
	cred.Persist = wincred.PersistLocalMachine
	cred.UserName = credentialUser
	return cred.Write()
}

func loadUsername() (string, bool) {
	cred, err := wincred.GetGenericCredential(credentialUsername)
	if err != nil || len(cred.CredentialBlob) == 0 {
		return "", false
	}
	return string(cred.CredentialBlob), true
}

func saveUsername(username string) error {
	cred := wincred.NewGenericCredential(credentialUsername)
	cred.CredentialBlob = []byte(username)
	cred.Persist = wincred.PersistLocalMachine
	cred.UserName = credentialUser
	return cred.Write()
}

// Duolingo API functions
func getUserInfo(jwt, username string) (map[string]interface{}, bool) {
	req, err := http.NewRequest("GET", duolingoAPIBase+"/users/"+username, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, false
	}
	return data, true
}

func checkJWTValidity(data map[string]interface{}) bool {
	_, ok := data["username"]
	return ok
}

func checkTodayTask(data map[string]interface{}) bool {
	extended, _ := data["streak_extended_today"].(bool)
	return extended
}

// Browser automation for login
func promptLogin() bool {
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
		return false
	}

	if err := saveToken(jwt); err != nil {
		return false
	}
	return true
}

// Windows notification functions
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

// Windows Task Scheduler functions
func taskExists() bool {
	cmd := exec.Command("schtasks", "/Query", "/TN", taskName)
	return cmd.Run() == nil
}

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

// Admin privilege check
func isAdmin() bool {
	cmd := exec.Command("net", "session")
	err := cmd.Run()
	return err == nil
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
			scheduleTime := os.Args[3]

			// Phase 1: Save credentials (no admin needed)
			if err := saveUsername(username); err != nil {
				fmt.Printf("❌ Failed to save username: %v\n", err)
				return
			}
			fmt.Printf("✓ Username set to: %s\n", username)

			fmt.Println("\n✓ Opening browser for login...")
			if !promptLogin() {
				fmt.Println("❌ Login failed. Please try again.")
				return
			}
			fmt.Println("✓ Login successful!")

			// Phase 2: Create scheduled task (requires admin)
			if !isAdmin() {
				fmt.Println("\n🔒 Creating scheduled task requires administrator privileges...")
				fmt.Println("   Relaunching as administrator...")
				exe, _ := os.Executable()
				cmd := exec.Command("powershell", "Start-Process", "-FilePath", fmt.Sprintf("'%s'", exe), "-ArgumentList", fmt.Sprintf("'settime','%s'", scheduleTime), "-Verb", "RunAs", "-Wait")
				if err := cmd.Run(); err != nil {
					fmt.Println("❌ Failed to elevate. Please run manually as admin:")
					fmt.Printf("   DuoHelper.exe settime %s\n", scheduleTime)
					return
				}
				fmt.Println("\n✅ Setup complete! DuoHelper will check your Duolingo progress daily.")
			} else {
				// Already admin, create task directly
				exe, err := os.Executable()
				if err != nil {
					fmt.Printf("❌ Failed to get executable path: %v\n", err)
					return
				}
				if err := createScheduledTask(exe, scheduleTime); err != nil {
					fmt.Printf("❌ Failed to create task: %v\n", err)
					return
				}
				fmt.Printf("✓ Daily reminder scheduled for %s\n", scheduleTime)
				fmt.Println("\n✅ Setup complete! DuoHelper will check your Duolingo progress daily.")
			}
			return

		case "settime":
			if len(os.Args) < 3 {
				fmt.Println("Usage: DuoHelper.exe settime HH:MM")
				return
			}
			exe, err := os.Executable()
			if err != nil {
				fmt.Printf("❌ Failed to get executable path: %v\n", err)
				return
			}
			if err := createScheduledTask(exe, os.Args[2]); err != nil {
				fmt.Printf("❌ Failed to update task: %v\n", err)
				fmt.Println("💡 Tip: Run PowerShell as Administrator to modify scheduled tasks")
				return
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
			if promptLogin() {
				fmt.Println("✓ Login successful!")
			} else {
				fmt.Println("❌ Login failed. Please try again.")
			}
			return

		case "help":
			fmt.Println("DuoHelper - Duolingo Daily Reminder")
			fmt.Println("\nSetup (first time):")
			fmt.Println("  setup <username> <time>    Complete setup (will prompt for admin)")
			fmt.Println("                             Example: DuoHelper.exe setup YourUsername 09:00")
			fmt.Println("\nCommands:")
			fmt.Println("  settime HH:MM              Change reminder time (requires admin)")
			fmt.Println("  setusername <username>     Change Duolingo username")
			fmt.Println("  login                      Re-login to refresh JWT token")
			fmt.Println("  help                       Show this help message")
			return

		default:
			fmt.Printf("Unknown command '%s'. Run 'DuoHelper.exe help' for usage.\n", os.Args[1])
			return
		}
	}

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

	data, ok := getUserInfo(jwt, username)
	if !ok {
		sendNotification("DuoHelper: Failed to fetch user info. Please run: DuoHelper.exe login")
		return
	}

	if !checkJWTValidity(data) {
		sendNotification("DuoHelper: Invalid JWT token. Please run: DuoHelper.exe login")
		return
	}

	if checkTodayTask(data) {
		sendNotification("You have already completed today's Duolingo lesson! ✅")
	} else {
		sendNotification("You have NOT completed today's Duolingo lesson! ❌")
	}
}
