package main

import (
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Global list to store enterprise names
var enterprises []string
var mu sync.Mutex // Mutex to handle concurrent access to the list

func main() {
	// Initialize the MQTT client
	client := initializeMQTTClient("tcp://localhost:1883")

	// Subscribe to the topic
	subscribeToTopic(client, "car/enterprises", messageHandler)

	go func() {
		for {
			PublishToEnterprise(client, "Hello from the car!")
			time.Sleep(1 * time.Second)
		}
	}()

	// Keep the program running
	select {}
}

// initializeMQTTClient initializes and connects an MQTT client
func initializeMQTTClient(broker string) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	return client
}

// subscribeToTopic subscribes to a given topic with a message handler
func subscribeToTopic(client mqtt.Client, topic string, handler mqtt.MessageHandler) {
	if token := client.Subscribe(topic, 0, handler); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
}

// messageHandler processes incoming messages from the subscribed topic
func messageHandler(client mqtt.Client, msg mqtt.Message) {
	enterprise := string(msg.Payload())
	addEnterprise(enterprise)
	fmt.Printf("Received and stored enterprise: %s\n", enterprise)
}

// addEnterprise adds an enterprise name to the global list
func addEnterprise(name string) {
	mu.Lock()
	defer mu.Unlock()
	enterprises = append(enterprises, name)
}

func PublishToEnterprise(client mqtt.Client, message string) {
	mu.Lock()
	defer mu.Unlock()

	for _, enterprise := range enterprises {
		fmt.Println(enterprise)
		token := client.Publish(enterprise, 0, false, message)
		token.Wait()
		if token.Error() != nil {
			fmt.Printf("Error publishing message: %v\n", token.Error())
		} else {
			fmt.Printf("Published message: %s to topic: %s\n", message, enterprise)
		}
	}
}
