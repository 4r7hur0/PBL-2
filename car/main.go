package main

import (
	"fmt"
	"time"

	"github.com/4r7hur0/PBL-2/schemas"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	// Initialize the MQTT client
	client := initializeMQTTClient("tcp://localhost:1883")

	CarID := generateCarID()
	fmt.Printf("Car ID: %s\n", CarID)

	// Channel to receive messages from the MQTT broker
	responseChannel := make(chan string)

	go func() {
		// Subscribe to the topic
		subscribeToTopic(client, "car/enterprises", messageHandler)
	}()

	// Go rounine for messages from topic carID
	go func() {
		subscribeToTopic(client, CarID, func(c mqtt.Client, m mqtt.Message) {
			responseChannel <- string(m.Payload())
		})
	}()

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

	for {
		origin, destination := ChooseTwoRandomCities()
		if origin == "" && destination == "" {
			fmt.Println("No cities available. Retrying in 5 seconds...")
			time.Sleep(5 * time.Second)
			continue
		}

		fmt.Printf("Origin: %s, Destination: %s\n", origin, destination)

		// Publish the charging request
		PublishChargingRequest(client, origin, destination, CarID, selectedEnterprise.Name)
		fmt.Println("Waiting for response...")
		// Wait for a response from the MQTT broker
		response := <-responseChannel
		fmt.Printf("Received response: %s\n", response)
		break
	}

}
