package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/4r7hur0/PBL-2/schemas"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	// Initialize the MQTT client
	client := initializeMQTTClient("tcp://localhost:1883")

	CarID := generateCarID()
	fmt.Printf("Car ID: %s\n", CarID)

	// Channel to receive messages from the MQTT broker
	responseChannel := make(chan schemas.RouteReservationOptions)

	go func() {
		// Subscribe to the topic
		subscribeToTopic(client, "car/enterprises", messageHandler)
	}()

	// Go rounine for messages from topic carID
	go func() {
		subscribeToTopic(client, CarID, func(c mqtt.Client, m mqtt.Message) {
			var resp schemas.RouteReservationOptions
			err := json.Unmarshal(m.Payload(), &resp)
			if err != nil {
				fmt.Printf("Error deserializing message: %v\n", err)
				return
			}
			responseChannel <- resp
		})
	}()

	// Initialize battery level and discharge rate
	batteryLevel := initializeBatteryLevel()
	dischargeRate := initializeDischargeRate()
	fmt.Printf("Battery level: %d%%\n", batteryLevel)
	fmt.Printf("Discharge rate: %s\n", dischargeRate)

	var selectedEnterprise *schemas.Enterprises
	for {
		selectedEnterprise = chooseRandomEnterprise()
		if selectedEnterprise != nil {
			fmt.Printf("Selected enterprise: %s\n", selectedEnterprise.Name)
			break
		} else {
			fmt.Println("No enterprise available. Retrying in 5 seconds...")
			time.Sleep(5 * time.Second)
		}
	}

	for {
		origin, destination := ChooseTwoRandomCities()
		if origin == "" && destination == "" {
			fmt.Println("No cities available. Retrying in 5 seconds...")
			time.Sleep(5 * time.Second)
			continue
		}

		fmt.Printf("Origin: %s, Destination: %s\n", origin, destination)

		// Publish the charging request
		PublishChargingRequest(client, origin, destination, CarID, selectedEnterprise.Name)
		fmt.Println("Waiting for response...")
		// Wait for a response from the MQTT broker
		// This is a blocking call, so it will wait until a message is received
	RETRY_ROUTE:
		response := <-responseChannel
		if len(response.Routes) == 0 {
			fmt.Println("No route available. Retrying in 5 seconds...")
			time.Sleep(5 * time.Second)
			goto RETRY_ROUTE
		}

		rand.Seed(time.Now().UnixNano())
		randomIndex := rand.Intn(len(response.Routes))
		selectedRoute := response.Routes[randomIndex]
				fmt.Println("\nRota escolhida:")
		if len(selectedRoute) == 0 {
			fmt.Println("  No route segments provided.")
		} else {
			for i, segment := range selectedRoute {
				start := segment.ReservationWindow.StartTimeUTC.Format("15:04")
				end := segment.ReservationWindow.EndTimeUTC.Format("15:04")
				date := segment.ReservationWindow.StartTimeUTC.Format("02/01/2006")

				fmt.Printf("  Etapa %d: Cidade: %s, Janela de Reserva: %s até %s - %s\n", i+1, segment.City, start, end, date)
			}
		}


		// Publish the route reservation
		chosenRouteMsg := schemas.ChosenRouteMsg{
			RequestID: response.RequestID,
			VehicleID: CarID,
			Route:     selectedRoute,
		}

		payload, err := json.Marshal(chosenRouteMsg)
		if err != nil {
			fmt.Printf("Error serializing message: %v\n", err)
			continue
		}

		token := client.Publish("car/route", 0, false, payload)
		token.Wait()
		if token.Error() != nil {
			fmt.Printf("Error publishing message: %v\n", token.Error())
			continue
		}
		//fmt.Printf("Route reservation published: %s\n", string(payload))
		fmt.Println("\nReserva de rota publicada:")

		for i, segment := range selectedRoute {
		start := segment.ReservationWindow.StartTimeUTC.Format("15:04")
		end := segment.ReservationWindow.EndTimeUTC.Format("15:04")
		date := segment.ReservationWindow.StartTimeUTC.Format("02/01/2006")
		fmt.Printf("  Etapa %d: %s | Janela: %s até %s - %s\n", i+1, segment.City, start, end, date)}


		fmt.Println("\nWaiting for response...")
		response = <-responseChannel
		fmt.Printf("Response received: %v\n", response)

	}

}
