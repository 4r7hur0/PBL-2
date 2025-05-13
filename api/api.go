package main

import (
	"log"
	"os"      // For environment variables
	"strconv" // For parsing total posts from env
	"strings" // For multiple company configurations
	"sync"    // For waiting if multiple services run
	
	"github.com/4r7hur0/PBL-2/api/mqtt" // Assuming this is the correct path for the MQTT package
	// Assuming local packages are correctly pathed relative to the Go module root
	companyservice "github.com/4r7hur0/PBL-2/api/router" // Renamed to avoid conflict with router package name
	"github.com/4r7hur0/PBL-2/registry"
	mqttPaho "github.com/eclipse/paho.mqtt.golang" // Alias for paho mqtt
)

// Config struct for a single company
type CompanyConfig struct {
	Name       string
	City       string
	TotalPosts int
	BrokerURL  string
	ClientID   string
}

func main() {
	defaultBrokerURL := "tcp://localhost:1883"

	

	// Example: Configure multiple companies via environment variables
	// COMPANY_CONFIGS="SolAltantico,Salvador,5,solaltantico_client;EnterpriseB,Feira de Santana,3,companyB_client"
	// Each company string: Name,City,TotalPosts,ClientID
	configsStr := os.Getenv("COMPANY_CONFIGS")
	if configsStr == "" {
		log.Println("COMPANY_CONFIGS environment variable not set. Using default single company setup.")
		// Default single company setup for easy testing
		configsStr = "SolAltantico,Salvador,5,solaltantico_api_client"
	}

	mqtt.StartListening(defaultBrokerURL, 10)

	companyConfigStrings := strings.Split(configsStr, ";")
	var companyConfigs []CompanyConfig

	for i, confStr := range companyConfigStrings {
		parts := strings.Split(confStr, ",")
		if len(parts) < 3 || len(parts) > 4 { // Name,City,Posts,[ClientID]
			log.Printf("Invalid format for company config string: '%s'. Expected Name,City,TotalPosts,[ClientID]. Skipping.", confStr)
			continue
		}
		name := strings.TrimSpace(parts[0])
		city := strings.TrimSpace(parts[1])
		totalPosts, err := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err != nil {
			log.Printf("Invalid TotalPosts for company %s ('%s'): %v. Skipping.", name, parts[2], err)
			continue
		}
		clientID := name + "_api_client_" + strconv.Itoa(i) // Default unique client ID
		if len(parts) == 4 && strings.TrimSpace(parts[3]) != "" {
			clientID = strings.TrimSpace(parts[3])
		}

		companyConfigs = append(companyConfigs, CompanyConfig{
			Name:       name,
			City:       city,
			TotalPosts: totalPosts,
			BrokerURL:  defaultBrokerURL, // Could also be part of config string
			ClientID:   clientID,
		})
	}

	if len(companyConfigs) == 0 {
		log.Fatal("No valid company configurations found. Exiting.")
	}

	// Initialize Service Registry (shared among all company services in this process)
	sr := registry.NewServiceRegistry()

	// Populate the registry with all companies being run by this API instance
	// In a distributed system, registry might be external or use a discovery service.
	for _, conf := range companyConfigs {
		// The registry stores info about where the company's *service* (e.g., HTTP for 2PC) might be.
		// For now, Host/Port are placeholders as we focus on MQTT.
		sr.RegisterEnterprise(registry.EnterpriseService{
			Name:     conf.Name,
			City:     conf.City,
			Capacity: conf.TotalPosts, // Initial capacity
			Host:     "localhost",     // Placeholder for potential HTTP service
			Port:     8080 + len(sr.GetAllEnterprises()), // Placeholder unique port
		})
	}
    // If this API needs to know about companies run by *other* API instances,
    // their details should also be in the registry (e.g., loaded from a shared config or DB).
    // For pathfinding, the registry should ideally contain ALL enterprises in the system.
    // Example: Add some external enterprises for broader pathfinding
    sr.RegisterEnterprise(registry.EnterpriseService{Name: "ExternalCompX", City: "Lencois", Capacity: 2, Host: "remotehost", Port: 9090})
    sr.RegisterEnterprise(registry.EnterpriseService{Name: "ExternalCompY", City: "Ilheus", Capacity: 4, Host: "anotherhost", Port: 9091})


	var wg sync.WaitGroup // To keep main alive if running multiple services as goroutines

	for _, config := range companyConfigs {
		wg.Add(1)
		go func(conf CompanyConfig) { // Launch each company service in its own goroutine
			defer wg.Done()

			// --- MQTT Client Setup for this Company Service ---
			opts := mqttPaho.NewClientOptions().AddBroker(conf.BrokerURL)
			opts.SetClientID(conf.ClientID) // Unique client ID for each company's API connection
			opts.SetCleanSession(true)      // Clean session for reliability
			opts.SetAutoReconnect(true)
			opts.SetConnectRetry(true)
			
			// Optional: OnConnect handler to re-subscribe if connection is lost and re-established
			var companyServ *companyservice.CompanyService // Declare to use in OnConnect
			opts.SetOnConnectHandler(func(client mqttPaho.Client) {
				log.Printf("MQTT client for Company [%s] (ClientID: %s) connected to broker.", conf.Name, conf.ClientID)
				// Re-subscribe on connect/reconnect
				if companyServ != nil { // companyServ is initialized after client connection
					companyRequestTopic := conf.Name
					if token := client.Subscribe(companyRequestTopic, 1, companyServ.HandleRouteRequest); token.Wait() && token.Error() != nil {
						log.Printf("Company [%s]: Failed to re-subscribe to topic [%s] on reconnect: %v", conf.Name, companyRequestTopic, token.Error())
					} else {
						log.Printf("Company [%s]: Re-subscribed to MQTT topic [%s] for route requests on reconnect.", conf.Name, companyRequestTopic)
					}
					// Also re-trigger listening for confirmed reservations, as the subscription might be lost.
					// The listenForConfirmedReservations itself handles the subscription.
					// We might need to ensure it's robust to reconnections or re-call it.
					// For simplicity, the initial call to listenForConfirmedReservations should use this robust client.
				}
			})
			opts.SetConnectionLostHandler(func(client mqttPaho.Client, err error) {
				log.Printf("MQTT client for Company [%s] (ClientID: %s) connection lost: %v", conf.Name, conf.ClientID, err)
			})


			companyMqttClient := mqttPaho.NewClient(opts)
			if token := companyMqttClient.Connect(); token.Wait() && token.Error() != nil {
				log.Printf("Failed to connect MQTT client for Company [%s] (ClientID: %s): %v. This service instance will not run.", conf.Name, conf.ClientID, token.Error())
				return // Skip this company service if MQTT connection fails
			}
			// If OnConnectHandler is used for subscriptions, initial subscriptions happen there.
			// Otherwise, subscribe after connection.

			log.Printf("Initializing API for Company: %s, City: %s, Posts: %d, MQTT ClientID: %s", conf.Name, conf.City, conf.TotalPosts, conf.ClientID)

			// Create and initialize the CompanyService
			// Pass the already connected companyMqttClient
			companyServ = companyservice.NewCompanyService(conf.Name, conf.City, conf.TotalPosts, sr, companyMqttClient)

			// Subscribe the company service to its dedicated MQTT topic for route requests
			// This is now handled by OnConnectHandler for robustness, but an initial sub is also good.
			companyRequestTopic := conf.Name
			if token := companyMqttClient.Subscribe(companyRequestTopic, 1, companyServ.HandleRouteRequest); token.Wait() && token.Error() != nil {
				log.Printf("Company [%s]: Failed to subscribe to topic [%s]: %v. Route requests might not be received.", conf.Name, companyRequestTopic, token.Error())
			} else {
				log.Printf("Company [%s]: Subscribed to MQTT topic [%s] for route requests.", conf.Name, companyRequestTopic)
			}
			// Note: listenForConfirmedReservations is called inside NewCompanyService and will use the same client.

			log.Printf("Company API for [%s] is running and listening for MQTT messages.", conf.Name)

		}(config)
	}

	// Keep the main goroutine alive until all company services are done (which is forever in this setup)
	wg.Wait()
	log.Println("All company services have been signaled to stop. Main application exiting.")
	
}
