package main

import (
	"encoding/json"
	"fmt"

	"github.com/4r7hur0/PBL-2/schemas"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	// Initialize the MQTT client
	client := initializeMQTTClient("tcp://localhost:1883")

	// Enterprises to publish
	enterprises := []schemas.Enterprises{
		{Name: "SolAltantico", City: "Salvador"},
		{Name: "SertaoCarga", City: "Feira de Santana"},
		{Name: "ChapadaEletric", City: "Lençois"},
		{Name: "CacauPower", City: "Ilheus"},
		{Name: "VelhoChicoEnergia", City: "Juazeiro"},
	}

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
func publishEnterprises(client mqtt.Client, topic string, enterprises []schemas.Enterprises) {
	for _, en := range enterprises {
		// Serialize the struct to JSON
		payload, err := json.Marshal(en)
		if err != nil {
			fmt.Printf("Error serializing enterprise: %v\n", err.Error())
			continue
		}

		token := client.Publish(topic, 0, false, payload)
		token.Wait()
		if token.Error() != nil {
			fmt.Printf("Error publishing message: %v\n", token.Error())
		} else {
			fmt.Printf("Published message: %s to topic: %s\n", en, topic)
		}
	}
}
