package main

import (
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// PublishToEnterprise publishes a message to all enterprises in the list
func PublishToEnterprise(client mqtt.Client, message string) {
	mu.Lock()
	defer mu.Unlock()

	for _, enterprise := range enterprises {
		// Publish the message to the formatted topic
		token := client.Publish(enterprise, 0, false, message)
		token.Wait()
		if token.Error() != nil {
			fmt.Printf("Error publishing message to %s: %v\n", enterprise, token.Error())
		} else {
			fmt.Printf("Published message: %s to topic: %s\n", message, enterprise)
		}
	}
}
