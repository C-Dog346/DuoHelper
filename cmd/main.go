package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-toast/toast"
)

// Tokens stores your JWT for Duolingo API access
type Tokens struct {
	JWT string `json:"jwt"`
}

// loadToken reads tokens.json and returns the JWT
func loadToken() string {
	b, err := os.ReadFile("tokens.json")
	if err != nil {
		log.Fatalf("Failed to read tokens.json: %v", err)
	}

	var t Tokens
	if err := json.Unmarshal(b, &t); err != nil {
		log.Fatalf("Invalid JSON in tokens.json: %v", err)
	}

	if t.JWT == "" {
		log.Fatal("JWT token is empty in tokens.json")
	}

	return t.JWT
}

// getUserInfo calls the Duolingo API and returns user data
func getUserInfo(jwt string) map[string]interface{} {
	req, err := http.NewRequest("GET", "https://www.duolingo.com/users/CallumClow5", nil)
	if err != nil {
		log.Fatalf("Failed to create HTTP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Fatalf("Failed to decode JSON response: %v", err)
	}

	return data
}

// sendNotification displays a Windows Toast notification
func sendNotification(message string) {
	notification := toast.Notification{
		AppID:   "Duolingo Notifier",
		Title:   "Duolingo Reminder",
		Message: message,
	}
	if err := notification.Push(); err != nil {
		log.Printf("Failed to send notification: %v", err)
	}
}

// checkTodayTask returns true if the streak has been extended today
func checkTodayTask(data map[string]interface{}) bool {
	extended, ok := data["streak_extended_today"].(bool)
	if !ok {
		log.Println("Warning: streak_extended_today not found, assuming false")
		return false
	}
	return extended
}

func main() {
	jwt := loadToken()
	data := getUserInfo(jwt)

	doneToday := checkTodayTask(data)

	if doneToday {
		fmt.Println("✔ You have already done your Duolingo task today!")
		sendNotification("You have already completed today's Duolingo lesson! ✅")
	} else {
		fmt.Println("❌ You have NOT done your Duolingo task today!")
		sendNotification("You have NOT completed today's Duolingo lesson! ❌")
	}
}
