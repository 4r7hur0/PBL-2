package registry

import (
	"bytes"
	"io"
	//"encoding/json"
	"fmt"
	"net/http"
	"sync"
	// "time" // Para timeout no http.Client
)

type EnterpriseService struct {
	Name            string   `json:"name"`
	Host            string   `json:"host"`
	Port            int      `json:"port"`
	City            string   `json:"city"`
	Capacity        int      `json:"capacity"`
	ChargingPoints  []string `json:"charging_points,omitempty"`
	// Opcional: Caminhos específicos para os endpoints 2PC se não forem padronizados
	// PrepareEndpoint string `json:"prepare_endpoint,omitempty"` // Ex: "/api/transaction/prepare"
	// CommitEndpoint  string `json:"commit_endpoint,omitempty"`
	// AbortEndpoint   string `json:"abort_endpoint,omitempty"`
}

type ServiceRegistry struct {
	enterprises map[string]EnterpriseService
	mu          sync.RWMutex
}

func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		enterprises: make(map[string]EnterpriseService),
	}
}

// Start a enterprise
func (sr *ServiceRegistry) RegisterEnterprise(enterprise EnterpriseService) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.enterprises[enterprise.Name] = enterprise
	fmt.Printf("Registry: Empresa registrada/atualizada: %s em %s:%d (Cidade: %s)\n",
		enterprise.Name, enterprise.Host, enterprise.Port, enterprise.City)
}

func (sr *ServiceRegistry) GetEnterpriseByName(name string) (EnterpriseService, bool) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	enterprise, exists := sr.enterprises[name]
	return enterprise, exists
}

func (sr *ServiceRegistry) GetEnterprisesByCity(city string) []EnterpriseService {
	sr.mu.RLock(); defer sr.mu.RUnlock()
	var result []EnterpriseService
	for _, enterprise := range sr.enterprises { if enterprise.City == city { result = append(result, enterprise) } }
	return result
}


func (sr *ServiceRegistry) GetAllEnterprises() []EnterpriseService {
	sr.mu.RLock(); defer sr.mu.RUnlock()
	result := make([]EnterpriseService, 0, len(sr.enterprises))
	for _, enterprise := range sr.enterprises { result = append(result, enterprise) }
	return result
}

// Make the path based on the route init and final cities
func (sr *ServiceRegistry) FindEnterprisesByPath(originCity, destinationCity string) []EnterpriseService {
	sr.mu.RLock(); defer sr.mu.RUnlock()
	var result []EnterpriseService
	for _, enterprise := range sr.enterprises { if enterprise.City != originCity && enterprise.City != destinationCity { result = append(result, enterprise) } }
	return result
}


// ContactEnterprise envia uma requisição HTTP para o serviço de outra empresa.
func ContactEnterprise(enterprise EnterpriseService, endpoint string, method string, payload []byte) ([]byte, error) {
	url := fmt.Sprintf("http://%s:%d%s", enterprise.Host, enterprise.Port, endpoint)
	fmt.Printf("Registry: Contactando %s (%s) - Método: %s, Payload: %s\n", enterprise.Name, url, method, string(payload))


	var req *http.Request
	var err error

	if payload != nil && len(payload) > 0 {
		req, err = http.NewRequest(method, url, bytes.NewBuffer(payload))
		if err != nil {
			return nil, fmt.Errorf("erro ao criar requisição HTTP para %s: %w", url, err)
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
		if err != nil {
			return nil, fmt.Errorf("erro ao criar requisição HTTP para %s: %w", url, err)
		}
	}

	client := &http.Client{
		// Timeout: 10 * time.Second, // Considerar adicionar timeout
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro ao enviar requisição para %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body) // ou io.ReadAll para Go 1.16+
	if err != nil {
		return nil, fmt.Errorf("erro ao ler corpo da resposta de %s: %w", url, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return body, fmt.Errorf("requisição para %s falhou com status %s (%d): %s",
			url, resp.Status, resp.StatusCode, string(body))
	}

	return body, nil
}