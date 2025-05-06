package main

import (
	"github.com/4r7hur0/PBL-2/api/mqtt"
	"github.com/4r7hur0/PBL-2/api/router"
)

func main() {
	// Initialize MQTT client
	mqtt.InitializeMQTT("tcp://localhost:1883")

	messageChannel := mqtt.StartListening("EnterpriseA", 10)

	go func() {
		for message := range messageChannel {
			// Process the incoming message
			println("Received message: ", message)
		}
	}()

	// Start the API router
	router.InitRouter()
}
