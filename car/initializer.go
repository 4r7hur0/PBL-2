package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
)

// Generate Car ID unique
func generateCarID() string {
	return uuid.New().String()
}

// Initialize Battery level the battery level with a random value
func initializeBatteryLevel() int {
	rand.Seed(time.Now().UnixNano())
	batteryLevel := rand.Intn(51) + 50 // Random value between 50 and 100
	fmt.Printf("Initialized battery level: %d%%\n", batteryLevel)
	return batteryLevel
}

// Initialize Discharge rate
func initializeDischargeRate() string {
	rand.Seed(time.Now().UnixNano())
	dischargeRate := rand.Intn(21) + 10 // Random value between 10 and 30
	fmt.Printf("Initialized discharge rate: %d%%\n", dischargeRate)
	return fmt.Sprintf("%d%%", dischargeRate)
}
