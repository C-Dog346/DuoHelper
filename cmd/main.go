package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

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

// promptLogin prompts the user to log in to Duolingo to obtain a new JWT token
func promptLogin() {

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
