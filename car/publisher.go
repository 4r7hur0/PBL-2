package main

import (
	"encoding/json"
	"fmt"

	"github.com/4r7hur0/PBL-2/schemas"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// PublishToEnterprise publishes a message to all enterprises in the list
func PublishChargingRequest(client mqtt.Client, topic string, request schemas.ChargingResquest) {
	// Serialize the request to JSON
	payload, err := json.Marshal(request)
	if err != nil {
		fmt.Printf("Error serializing request: %v\n", err)
		return
	}

	// Publish the message to the topic
	token := client.Publish(topic, 0, false, payload)
	token.Wait()
	if token.Error() != nil {
		fmt.Printf("Error publishing message: %v\n", token.Error())
		return
	}
	fmt.Printf("Published message to topic %s: %s\n", topic, string(payload))
}
