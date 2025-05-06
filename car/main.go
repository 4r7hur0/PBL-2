package main

import (
	"time"
)

func main() {
	// Initialize the MQTT client
	client := initializeMQTTClient("tcp://localhost:1883")

	// Subscribe to the topic
	subscribeToTopic(client, "car/enterprises", messageHandler)

	// Start publishing messages to enterprises
	go func() {
		for {
			PublishToEnterprise(client, "Hello from the car!")
			time.Sleep(1 * time.Second)
		}
	}()

	// Keep the program running
	select {}
}
