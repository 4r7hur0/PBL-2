package router

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/4r7hur0/PBL-2/registry"
	"github.com/4r7hur0/PBL-2/schemas"
	mqttClient "github.com/eclipse/paho.mqtt.golang"
)

type CompanyService struct {
	CompanyName     string
	ManagedCity     string
	TotalPosts      int
	AvailablePosts  int
	mu              sync.Mutex 
	ServiceRegistry *registry.ServiceRegistry
	MqttClient      mqttClient.Client
	allEnterprises  []registry.EnterpriseService 
}

// NewCompanyService creates a new CompanyService instance.
// It also starts a listener for confirmed reservations to update post counts.
func NewCompanyService(name, city string, totalPosts int, sr *registry.ServiceRegistry, client mqttClient.Client) *CompanyService {
	cs := &CompanyService{
		CompanyName:     name,
		ManagedCity:     city,
		TotalPosts:      totalPosts,
		AvailablePosts:  totalPosts,
		ServiceRegistry: sr,
		MqttClient:      client,
	}
	// Cache all enterprises from the registry for pathfinding
	if sr != nil {
		cs.allEnterprises = sr.GetAllEnterprises()
	} else {
		cs.allEnterprises = []registry.EnterpriseService{}
		log.Printf("Warning: CompanyService for %s initialized with a nil ServiceRegistry.", name)
	}

	// Start listening for confirmed reservations to update post count
	go cs.listenForConfirmedReservations()
	return cs
}

// HandleRouteRequest is the MQTT message handler for incoming route requests on the company's topic.
func (cs *CompanyService) HandleRouteRequest(client mqttClient.Client, msg mqttClient.Message) {
	log.Printf("Company [%s]: Received route request on topic [%s]", cs.CompanyName, msg.Topic())
	var req schemas.RouteRequest
	if err := json.Unmarshal(msg.Payload(), &req); err != nil {
		log.Printf("Company [%s]: Error unmarshalling RouteRequest: %v. Payload: %s", cs.CompanyName, err, string(msg.Payload()))
		return
	}

	log.Printf("Company [%s]: Processing RouteRequest for CarID [%s], Origin [%s], Destination [%s]", cs.CompanyName, req.VehicleID, req.Origin, req.Destination)

	// Find possible paths (returns a list of paths, where each path is []schemas.RouteSegment)
	foundPaths := cs.findPaths(req.Origin, req.Destination)

	if len(foundPaths) == 0 {
		log.Printf("Company [%s]: No routes found for CarID [%s] from [%s] to [%s]", cs.CompanyName, req.VehicleID, req.Origin, req.Destination)
		// Optionally send a response with an empty route list to indicate no routes found
		emptyResponse := schemas.RouteReservationRespose{
			RequestID: fmt.Sprintf("NO_ROUTE_%s_%d", req.VehicleID, time.Now().UnixNano()),
			VehicleID: req.VehicleID,
			Route:     []schemas.RouteSegment{},
		}
		payload, err := json.Marshal(emptyResponse)
		if err != nil {
			log.Printf("Company [%s]: Error marshalling empty route response: %v", cs.CompanyName, err)
			return
		}
		carTopic := req.VehicleID
		token := cs.MqttClient.Publish(carTopic, 0, false, payload)
		token.Wait() // Wait for publish to complete
		if token.Error() != nil {
			log.Printf("Company [%s]: Error publishing empty route response to topic [%s]: %v", cs.CompanyName, carTopic, token.Error())
		} else {
			log.Printf("Company [%s]: Published empty route response to topic [%s] for CarID [%s]", cs.CompanyName, carTopic, req.VehicleID)
		}
		return
	}

	// For now, let's send the first found path.
	// A more advanced implementation might send multiple RouteReservationRespose messages,
	// or a single message with a list of paths if the schema supported it.
	// Based on current car code, it expects one RouteReservationRespose with one path.
	firstPath := foundPaths[0]
	response := schemas.RouteReservationRespose{
		RequestID: fmt.Sprintf("ROUTE_RESP_%s_%d", req.VehicleID, time.Now().UnixNano()),
		VehicleID: req.VehicleID,
		Route:     firstPath, // Route is []schemas.RouteSegment representing one path
	}

	payload, err := json.Marshal(response)
	if err != nil {
		log.Printf("Company [%s]: Error marshalling RouteReservationRespose: %v", cs.CompanyName, err)
		return
	}

	carTopic := req.VehicleID
	token := cs.MqttClient.Publish(carTopic, 0, false, payload)
	token.Wait() // Wait for publish to complete
	if token.Error() != nil {
		log.Printf("Company [%s]: Error publishing route response to topic [%s]: %v", cs.CompanyName, carTopic, token.Error())
	} else {
		log.Printf("Company [%s]: Published route options to topic [%s] for CarID [%s]. Path: %+v", cs.CompanyName, carTopic, req.VehicleID, firstPath)
	}
}

// findPaths finds all possible sequences of cities (routes) from origin to destination.
// Each route is represented as a slice of RouteSegment.
func (cs *CompanyService) findPaths(origin, destination string) [][]schemas.RouteSegment {
	var allFoundPaths [][]schemas.RouteSegment
	
	// Build an adjacency list for pathfinding.
	// Nodes are cities that have registered enterprises.
	adj := make(map[string][]string)
	cityToEnterpriseMap := make(map[string]bool) // To ensure we only consider cities with services

	for _, ent := range cs.allEnterprises {
		cityToEnterpriseMap[ent.City] = true
	}

	// Basic connectivity: assume any city with an enterprise can connect to any other city with an enterprise.
	// This is a simplification; a real system needs actual road network data.
	// For DFS, we need to know which cities are valid stops (have enterprises).
	// Edges in 'adj' should represent direct travel possibility.
	// For this example, we'll assume a conceptual "can travel to any other service city".
	// The DFS will explore paths through these service cities.
	
	// Populate adj: For each city with an enterprise, list other cities with enterprises as neighbors.
	// This creates a fully connected graph of service cities, which is not realistic for roads
	// but allows DFS to find sequences.
	var serviceCities []string
	for city := range cityToEnterpriseMap {
		serviceCities = append(serviceCities, city)
	}

	for _, city1 := range serviceCities {
		for _, city2 := range serviceCities {
			if city1 != city2 {
				adj[city1] = append(adj[city1], city2)
			}
		}
	}
    // Ensure origin and destination are part of the graph if they have services
    if !cityToEnterpriseMap[origin] {
        log.Printf("Company [%s]: Origin city [%s] has no registered enterprise. Cannot start pathfinding.", cs.CompanyName, origin)
        return nil
    }
    if !cityToEnterpriseMap[destination] {
         log.Printf("Company [%s]: Destination city [%s] has no registered enterprise. Cannot end path.", cs.CompanyName, destination)
        return nil
    }


	var currentPathCities []string
	visited := make(map[string]bool)

	log.Printf("Company [%s]: Starting DFS from Origin: %s, Dest: %s", cs.CompanyName, origin, destination)
	cs.dfs(origin, destination, adj, &currentPathCities, &allFoundPaths, visited, cityToEnterpriseMap)
	log.Printf("Company [%s]: DFS finished. Found %d paths.", cs.CompanyName, len(allFoundPaths))

	return allFoundPaths
}

// dfs (Depth First Search) helper to find paths.
// currentPathCities stores city names.
// allPaths stores lists of RouteSegments.
func (cs *CompanyService) dfs(currentCity, destinationCity string, adj map[string][]string,
	currentPathCities *[]string, allPaths *[][]schemas.RouteSegment, visited map[string]bool, cityToEnterpriseMap map[string]bool) {

	if !cityToEnterpriseMap[currentCity] { // Should not happen if graph is built from serviceCities
		return
	}

	visited[currentCity] = true
	*currentPathCities = append(*currentPathCities, currentCity)

	if currentCity == destinationCity {
		// Found a path (sequence of city names). Convert to []schemas.RouteSegment.
		var segments []schemas.RouteSegment
		dummyWindow := schemas.ReservationWindow{ // TODO: Implement proper reservation window logic
			StartTimeUTC: time.Now().Add(1 * time.Hour).Format(schemas.ISOFormat),
			EndTimeUTC:   time.Now().Add(2 * time.Hour).Format(schemas.ISOFormat),
		}
		validPath := true
		for _, cityInPath := range *currentPathCities {
			if !cs.companyExistsInCity(cityInPath) { // Double check, though DFS explores known service cities
				log.Printf("Company [%s]: City [%s] in discovered path has no company. Invalidating path.", cs.CompanyName, cityInPath)
				validPath = false
				break
			}
			// Check if this company (cs) manages cityInPath and has posts
			availableHere := false
			if cityInPath == cs.ManagedCity {
				cs.mu.Lock()
				if cs.AvailablePosts > 0 {
					availableHere = true
				}
				cs.mu.Unlock()
				log.Printf("Company [%s]: Path segment in own city [%s], available: %t", cs.CompanyName, cityInPath, availableHere)
			} else {
				// For other cities, we assume posts might be available. Car will verify.
				// The fact that it's in cityToEnterpriseMap means a company exists there.
				availableHere = true // Placeholder for "company exists"
				log.Printf("Company [%s]: Path segment in other city [%s]", cs.CompanyName, cityInPath)
			}
			
			// We add the segment regardless of current local availability for "other" cities,
			// as the car will do the actual reservation. For "own" city, it's an indication.
			// The problem asks for "possible routes".
			segments = append(segments, schemas.RouteSegment{City: cityInPath, ReservationWindow: dummyWindow})
		}
		
		if validPath && len(segments) > 0 {
			// Ensure the path makes sense (e.g., starts at origin, ends at dest)
			// The DFS ensures this by construction of currentPathCities.
			*allPaths = append(*allPaths, deepCopySegments(segments))
			log.Printf("Company [%s]: DFS found a valid path: %v", cs.CompanyName, segmentsToCityNames(segments))
		} else if !validPath {
             log.Printf("Company [%s]: DFS found an invalid path due to missing company: %v", cs.CompanyName, *currentPathCities)
        }
	} else {
		// Explore neighbors
		for _, neighbor := range adj[currentCity] {
			if !visited[neighbor] {
				cs.dfs(neighbor, destinationCity, adj, currentPathCities, allPaths, visited, cityToEnterpriseMap)
			}
		}
	}

	// Backtrack
	*currentPathCities = (*currentPathCities)[:len(*currentPathCities)-1]
	visited[currentCity] = false
}

// deepCopySegments creates a deep copy of a slice of RouteSegment.
func deepCopySegments(original []schemas.RouteSegment) []schemas.RouteSegment {
	cpy := make([]schemas.RouteSegment, len(original))
	copy(cpy, original)
	return cpy
}

// segmentsToCityNames is a helper for logging.
func segmentsToCityNames(segments []schemas.RouteSegment) []string {
    names := make([]string, len(segments))
    for i, s := range segments {
        names[i] = s.City
    }
    return names
}

// companyExistsInCity checks if any registered enterprise operates in the given city.
func (cs *CompanyService) companyExistsInCity(city string) bool {
	for _, ent := range cs.allEnterprises {
		if ent.City == city {
			return true
		}
	}
	return false
}

// listenForConfirmedReservations subscribes to a topic (e.g., "car/route")
// to listen for ChosenRouteMsg messages. If a segment in the chosen route
// is for this company's managed city, it decrements the available post count.
func (cs *CompanyService) listenForConfirmedReservations() {
	confirmationTopic := "car/route" // Topic where cars publish their chosen route/segment

	// Ensure MqttClient is not nil before subscribing
	if cs.MqttClient == nil {
		log.Printf("Company [%s]: MqttClient is nil, cannot subscribe to %s", cs.CompanyName, confirmationTopic)
		return
	}
	
	token := cs.MqttClient.Subscribe(confirmationTopic, 1, func(client mqttClient.Client, msg mqttClient.Message) {
		log.Printf("Company [%s]: Received message on [%s] for potential reservation update.", cs.CompanyName, msg.Topic())
		var chosenRoute schemas.ChosenRouteMsg
		if err := json.Unmarshal(msg.Payload(), &chosenRoute); err != nil {
			log.Printf("Company [%s]: Error unmarshalling ChosenRouteMsg from topic [%s]: %v. Payload: %s", cs.CompanyName, msg.Topic(), err, string(msg.Payload()))
			return
		}

		// A ChosenRouteMsg contains segments for the route the car is committing to.
		// The car's current implementation sends one segment at a time in ChosenRouteMsg.Route.
		for _, segment := range chosenRoute.Route {
			if segment.City == cs.ManagedCity {
				cs.mu.Lock()
				if cs.AvailablePosts > 0 {
					cs.AvailablePosts--
					log.Printf("Company [%s]: Post reserved in city [%s] for VehicleID [%s] via ChosenRouteMsg. Available posts: %d/%d",
						cs.CompanyName, cs.ManagedCity, chosenRoute.VehicleID, cs.AvailablePosts, cs.TotalPosts)
				} else {
					log.Printf("Company [%s]: WARNING - Attempted to reserve post in city [%s] (VehicleID: %s), but no posts were available. Available: %d/%d. This might indicate a race condition or stale info.",
						cs.CompanyName, cs.ManagedCity, chosenRoute.VehicleID, cs.AvailablePosts, cs.TotalPosts)
				}
				cs.mu.Unlock()
				// Assuming one car reserves at most one post in this city per specific ChosenRouteMsg.
				// If the ChosenRouteMsg could contain multiple segments for the same city (unlikely for this problem),
				// this logic might need adjustment. Given car sends one segment, this is fine.
				break // Found this company's city in the chosen segment(s).
			}
		}
	})
    token.Wait() // Wait for subscription to complete
	if token.Error() != nil {
        log.Printf("Company [%s]: Failed to subscribe to topic [%s]: %v", cs.CompanyName, confirmationTopic, token.Error())
    } else {
        log.Printf("Company [%s]: Subscribed to MQTT topic [%s] for reservation updates.", cs.CompanyName, confirmationTopic)
    }
}
