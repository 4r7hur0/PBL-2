package main

import (
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	// Initialize the MQTT client
	client := initializeMQTTClient("tcp://localhost:1883")

	// Enterprises to publish
	enterprises := []string{"empresaA", "empresaB", "empresaC"}
	topic := "car/enterprises"

	// Publish enterprises to the topic
	publishEnterprises(client, topic, enterprises)
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

// publishEnterprises publishes a list of enterprises to a given topic
func publishEnterprises(client mqtt.Client, topic string, enterprises []string) {
	for _, en := range enterprises {
		token := client.Publish(topic, 0, false, en)
		token.Wait()
		if token.Error() != nil {
			fmt.Printf("Error publishing message: %v\n", token.Error())
		} else {
			fmt.Printf("Published message: %s to topic: %s\n", en, topic)
		}
	}
}
