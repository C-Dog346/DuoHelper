// go build -o bin\DuoHelper.exe .\cmd\main.go
// TO DO:
// - Improve error handling and logging
// - Add unit tests for functions
// - Add support for other OS (macOS, Linux)

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"syscall"
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
func loadToken() (string, error) {
	cred, err := wincred.GetGenericCredential(credentialJWT)
	if err != nil {
		return "", err
	}
	if len(cred.CredentialBlob) == 0 {
		return "", fmt.Errorf("JWT token is empty")
	}
	return string(cred.CredentialBlob), nil
}

func saveToken(jwt string) error {
	cred := wincred.NewGenericCredential(credentialJWT)
	cred.CredentialBlob = []byte(jwt)
	cred.Persist = wincred.PersistLocalMachine
	cred.UserName = credentialUser
	return cred.Write()
}

func loadUsername() (string, error) {
	cred, err := wincred.GetGenericCredential(credentialUsername)
	if err != nil {
		return "", err
	}
	if len(cred.CredentialBlob) == 0 {
		return "", fmt.Errorf("username is empty")
	}
	return string(cred.CredentialBlob), nil
}

func saveUsername(username string) error {
	cred := wincred.NewGenericCredential(credentialUsername)
	cred.CredentialBlob = []byte(username)
	cred.Persist = wincred.PersistLocalMachine
	cred.UserName = credentialUser
	return cred.Write()
}

// Duolingo API functions
func getUserInfo(jwt, username string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", duolingoAPIBase+"/users/"+username, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return data, nil
}

func checkJWTValidity(data map[string]interface{}) bool {
	_, ok := data["username"] // check for presence of expected field
	return ok
}

func checkTodayTask(data map[string]interface{}) bool {
	extended, _ := data["streak_extended_today"].(bool)
	return extended
}

// Browser automation for login
func promptLogin() error {
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

	if err != nil {
		return fmt.Errorf("browser automation failed: %w", err)
	}
	if jwt == "" {
		return fmt.Errorf("no JWT token found")
	}

	if err := saveToken(jwt); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}
	return nil
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

func createScheduledTask(exePath, scheduleTime, runAsUser string) error {
	cmd := exec.Command("schtasks",
		"/Create", "/SC", "DAILY", "/TN", taskName,
		"/TR", fmt.Sprintf(`"%s"`, exePath), "/ST", scheduleTime,
		"/RU", runAsUser, "/F", "/RL", "HIGHEST", "/IT")

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

// Hide console window (for silent scheduled runs)
func hideConsoleWindow() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")

	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	showWindow := user32.NewProc("ShowWindow")

	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		showWindow.Call(hwnd, 0) // SW_HIDE = 0
	}
}

func main() {
	// Hide console window when running in silent mode (scheduled task)
	if len(os.Args) == 1 {
		hideConsoleWindow()
	}

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
			if err := promptLogin(); err != nil {
				fmt.Printf("❌ Login failed: %v\n", err)
				return
			}
			fmt.Println("✓ Login successful!")

			// Phase 2: Create scheduled task (requires admin)
			// Capture current user before potential elevation
			currentUser := os.Getenv("USERNAME")
			currentDomain := os.Getenv("USERDOMAIN")
			runAsUser := fmt.Sprintf("%s\\%s", currentDomain, currentUser)

			if !isAdmin() {
				fmt.Println("\n🔒 Creating scheduled task requires administrator privileges...")
				fmt.Println("   Relaunching as administrator...")
				exe, _ := os.Executable()
				cmd := exec.Command("powershell", "Start-Process", "-FilePath", fmt.Sprintf("'%s'", exe), "-ArgumentList", fmt.Sprintf("'settime','%s','%s'", scheduleTime, runAsUser), "-Verb", "RunAs", "-Wait")
				if err := cmd.Run(); err != nil {
					fmt.Println("❌ Failed to elevate. Please run manually as admin:")
					fmt.Printf("   DuoHelper.exe settime %s %s\n", scheduleTime, runAsUser)
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
				if err := createScheduledTask(exe, scheduleTime, runAsUser); err != nil {
					fmt.Printf("❌ Failed to create task: %v\n", err)
					return
				}
				fmt.Printf("✓ Daily reminder scheduled for %s\n", scheduleTime)
				fmt.Println("\n✅ Setup complete! DuoHelper will check your Duolingo progress daily.")
			}
			return

		case "settime":
			if len(os.Args) < 3 {
				fmt.Println("Usage: DuoHelper.exe settime HH:MM [DOMAIN\\USER]")
				return
			}
			exe, err := os.Executable()
			if err != nil {
				fmt.Printf("❌ Failed to get executable path: %v\n", err)
				return
			}

			// Use provided user or fall back to current user
			runAsUser := ""
			if len(os.Args) >= 4 {
				runAsUser = os.Args[3]
			} else {
				username := os.Getenv("USERNAME")
				domain := os.Getenv("USERDOMAIN")
				runAsUser = fmt.Sprintf("%s\\%s", domain, username)
			}

			if err := createScheduledTask(exe, os.Args[2], runAsUser); err != nil {
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
			if err := promptLogin(); err != nil {
				fmt.Printf("❌ Login failed: %v\n", err)
			} else {
				fmt.Println("✓ Login successful!")
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

	username, err := loadUsername()
	if err != nil {
		sendNotification("DuoHelper setup incomplete. Please run: DuoHelper.exe setup <username> <time>")
		return
	}

	if !taskExists() {
		sendNotification("DuoHelper scheduled task not found. Please run setup again.")
		return
	}

	jwt, err := loadToken()
	if err != nil {
		sendNotification("DuoHelper: JWT token missing. Please run: DuoHelper.exe login")
		return
	}

	data, err := getUserInfo(jwt, username)
	if err != nil {
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
