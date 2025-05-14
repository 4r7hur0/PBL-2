package main

import (
	"fmt"
	"time"

	"github.com/4r7hur0/PBL-2/schemas"
)

func main() {
	// Initialize the MQTT client
	client := initializeMQTTClient("tcp://localhost:1883")

	// Subscribe to the topic
	subscribeToTopic(client, "car/enterprises", messageHandler)

	CarID := generateCarID()
	fmt.Printf("Car ID: %s\n", CarID)

	// Initialize battery level and discharge rate
	batteryLevel := initializeBatteryLevel()
	dischargeRate := initializeDischargeRate()
	fmt.Printf("Battery level: %d%%\n", batteryLevel)
	fmt.Printf("Discharge rate: %s\n", dischargeRate)

	var selectedEnterprise *schemas.Enterprises
	for {
		selectedEnterprise = chooseRandomEnterprise()
		if selectedEnterprise != nil {
			fmt.Printf("Selected enterprise: %s\n", selectedEnterprise.Name)
			break
		} else {
			fmt.Println("No enterprise available. Retrying in 5 seconds...")
			time.Sleep(5 * time.Second)
		}
	}

	// escolher uma rota com base nas cidades disponiveis

	//esperar confirmação da api

	// Keep the program running
	select {}
}
