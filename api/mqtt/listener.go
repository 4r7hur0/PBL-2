package mqtt

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var MessageChannel = make(chan string, 10)

func StartListening(topic string) {

	// Subscribe to the specified topic
	go func() {
		Subscribe(topic, func(client mqtt.Client, msg mqtt.Message) {
			message := string(msg.Payload())
			//fmt.Printf("Received message: %s from topic: %s\n", message, msg.Topic())
			MessageChannel <- message
		})
	}()

}
