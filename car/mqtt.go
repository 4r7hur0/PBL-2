package main

import (
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// initializeMQTTClient initializes and connects an MQTT client
func initializeMQTTClient(broker string) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	fmt.Println("MQTT client connected to broker:", broker)
	return client
}

// subscribeToTopic subscribes to a given topic with a message handler
func subscribeToTopic(client mqtt.Client, topic string, handler mqtt.MessageHandler) {
	if token := client.Subscribe(topic, 0, handler); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	fmt.Printf("Subscribed to topic: %s\n", topic)
}
