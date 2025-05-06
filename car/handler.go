package main

import (
	"fmt"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Global list to store enterprise names
var enterprises []string
var mu sync.Mutex // Mutex to handle concurrent access to the list

// messageHandler processes incoming messages from the subscribed topic
func messageHandler(client mqtt.Client, msg mqtt.Message) {
	enterprise := string(msg.Payload())
	addEnterprise(enterprise)
	fmt.Printf("Received and stored enterprise: %s\n", enterprise)
}

// addEnterprise adds an enterprise name to the global list and prints the list
func addEnterprise(name string) {
	mu.Lock()
	defer mu.Unlock()
	enterprises = append(enterprises, name)
}
