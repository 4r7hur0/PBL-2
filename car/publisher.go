package main

import (
	"encoding/json"
	"fmt"

	"github.com/4r7hur0/PBL-2/schemas"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// PublishToEnterprise publishes a message to all enterprises in the list
func PublishChargingRequest(client mqtt.Client, origin, destination, carID, nameEnter, dischargeRate, topic string, batteryLevel int) {
	request := schemas.ChargingResquest{
		EnterpriseName:  nameEnter,
		CarID:           carID,
		BatteryLevel:    batteryLevel,
		DischargeRate:   dischargeRate,
		OriginCity:      origin,
		DestinationCity: destination,
	}

	payload, err := json.Marshal(request)
	if err != nil {
		fmt.Printf("Error serializing request: %v\n", err)
		return
	}
	token := client.Publish(topic, 0, false, payload)
	token.Wait()
	if token.Error() != nil {
		fmt.Printf("Error publishing message: %v\n", token.Error())
	} else {
		fmt.Printf("Published message: %v to topic: %v\n", request, topic)
	}
}
