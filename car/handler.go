package main

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/4r7hur0/PBL-2/schemas"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Global list to store enterprise names
var enterprises []schemas.Enterprises
var mu sync.Mutex // Mutex to handle concurrent access to the list

// messageHandler processes incoming messages from the subscribed topic
func messageHandler(client mqtt.Client, msg mqtt.Message) {
	var enterprise schemas.Enterprises

	err := json.Unmarshal(msg.Payload(), &enterprise)
	if err != nil {
		fmt.Printf("Error deserializing message: %v\n", err)
		return
	}
	addEnterprise(enterprise)
	fmt.Printf("Received and stored enterprise: %v\n", enterprise)
}

// addEnterprise adds an enterprise name to the global list and prints the list
func addEnterprise(enterprise schemas.Enterprises) {
	mu.Lock()
	defer mu.Unlock()
	enterprises = append(enterprises, enterprise)
}
