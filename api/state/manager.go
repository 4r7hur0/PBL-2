// state/manager.go
package state

import (
	"sync"
)

// StateManager holds the application's shared state.
type StateManager struct {
	cityAvailablePostsCount map[string]int
	preparedTransactions    map[string][]string 
	mutex                   *sync.Mutex
}

func NewStateManager(allCities []string, initialPostsPerCity int) *StateManager {
	manager := &StateManager{
		cityAvailablePostsCount: make(map[string]int),
		preparedTransactions:    make(map[string][]string),
		mutex:                   &sync.Mutex{},
	}

	manager.mutex.Lock()
	for _, city := range allCities {
		manager.cityAvailablePostsCount[city] = initialPostsPerCity
	}
	manager.mutex.Unlock()

	return manager
}

func (m *StateManager) PrepareReservation(transactionID string, cities []string) (bool, []string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	preparedCitiesForThisTx := []string{}
	prepareSuccess := true

	for _, city := range cities {
		if m.cityAvailablePostsCount[city] > 0 {
			m.cityAvailablePostsCount[city]--
			preparedCitiesForThisTx = append(preparedCitiesForThisTx, city)
		} else {
			prepareSuccess = false
			break 
		}
	}

	if prepareSuccess {
		m.preparedTransactions[transactionID] = preparedCitiesForThisTx
	} else {
		for _, cityToRollback := range preparedCitiesForThisTx {
			m.cityAvailablePostsCount[cityToRollback]++
		}
	}

	return prepareSuccess, preparedCitiesForThisTx
}

func (m *StateManager) CommitReservation(transactionID string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.preparedTransactions, transactionID)
}

func (m *StateManager) AbortReservation(transactionID string, preparedCities []string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, cityToRollback := range preparedCities {
		m.cityAvailablePostsCount[cityToRollback]++
	}
	delete(m.preparedTransactions, transactionID)
}

func (m *StateManager) GetTotalAvailability() map[string]int {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	availabilityCopy := make(map[string]int)
	for city, count := range m.cityAvailablePostsCount {
		availabilityCopy[city] = count
	}
	return availabilityCopy
}

func (m *StateManager) GetCityAvailability(city string) (int, bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	availability, ok := m.cityAvailablePostsCount[city]
	return availability, ok
}

func (m *StateManager) GetAllCities() []string {

  return []string{} 
}