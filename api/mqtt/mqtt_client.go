package mqtt

import (
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	// Handler para confirmações de publicação (opcional)
	log.Printf("Received confirmation for message: %s from topic: %s\n", msg.Payload(), msg.Topic())
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	log.Println("MQTT Connected")
	// Re-subscribe on connect/reconnect (important for reliability)
	optsReader := client.OptionsReader()
	if optsReader.AutoReconnect() {
		// Se a reconexão automática estiver ativa, é bom re-inscrever-se
		// A lógica de inscrição está em StartListening, então precisa ser chamada novamente
		// ou o cliente precisa ser reinicializado. Para simplificar, assumimos
		// que a inscrição inicial é suficiente ou que a reconexão lida com isso.
		// Em um cenário robusto, você gerenciaria as inscrições aqui.
		log.Println("MQTT Reconnected, ensure subscriptions are active.")
	}
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	log.Printf("MQTT Connection lost: %v. Attempting to reconnect...", err)
}

// InitializeMQTT inicializa e retorna um cliente MQTT.
func InitializeMQTT(brokerURL string, clientID string) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetClientID(clientID)
	opts.SetDefaultPublishHandler(messagePubHandler)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler
	opts.SetAutoReconnect(true)        // Habilita reconexão automática
	opts.SetConnectRetry(true)         // Tenta reconectar se a conexão inicial falhar
	opts.SetResumeSubs(true)				 // Retoma as inscrições após reconexão
	opts.SetConnectTimeout(10 * time.Second) // Aumenta o timeout de conexão
	opts.SetWriteTimeout(5 * time.Second)   // Timeout para escrita
	opts.SetKeepAlive(60 * time.Second)    // Keep alive para detectar conexões perdidas
	opts.SetPingTimeout(10 * time.Second)  // Timeout para resposta do ping
	opts.SetMaxReconnectInterval(1 * time.Minute) // Intervalo máximo entre tentativas de reconexão
	opts.SetCleanSession(true) // Começa com sessão limpa (sem mensagens persistentes QoS 1/2)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.WaitTimeout(15*time.Second) && token.Error() != nil { // Aumenta espera
		log.Fatalf("Failed to connect to MQTT broker: %s, Error: %v", brokerURL, token.Error())
	}
	if !client.IsConnected() {
		log.Fatalf("MQTT client failed to connect after timeout: %s", brokerURL)
	}
	log.Printf("Successfully connected to MQTT broker: %s with ClientID: %s", brokerURL, clientID)
	return client
}

// StartListening se inscreve em um tópico e retorna um canal para receber mensagens.
func StartListening(client mqtt.Client, topic string, bufferSize int) <-chan mqtt.Message {
	messageChannel := make(chan mqtt.Message, bufferSize)

	// Define o callback que será executado quando uma mensagem chegar
	var messageHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
		// Envia a mensagem para o canal de forma não bloqueante
		select {
		case messageChannel <- msg:
			// log.Printf("Message queued from topic %s", msg.Topic())
		default:
			log.Printf("Warning: Message channel full for topic %s. Message dropped.", topic)
		}
	}

	// Realiza a inscrição
	if token := client.Subscribe(topic, 1, messageHandler); token.WaitTimeout(10*time.Second) && token.Error() != nil {
		log.Printf("Warning: Failed to subscribe to MQTT topic %s: %v. Will retry implicitly if AutoReconnect is on.", topic, token.Error())
		// Não fataliza, pois AutoReconnect pode resolver.
	} else if token.Error() == nil {
		log.Printf("Subscribed to MQTT topic: %s", topic)
	} else {
		// Timeout ocorreu
		log.Printf("Warning: Timeout subscribing to MQTT topic %s.", topic)
	}


	return messageChannel
}

// PublishMessage publica uma mensagem em um tópico MQTT.
func PublishMessage(client mqtt.Client, topic string, payload string) error {
	if !client.IsConnected() {
		log.Printf("Error: Cannot publish message to %s. MQTT client not connected.", topic)
		return fmt.Errorf("MQTT client not connected")
	}
	token := client.Publish(topic, 0, false, payload) // QoS 0, não retido
	// Não esperar indefinidamente pela confirmação de QoS 0
	// token.WaitTimeout(2 * time.Second) // Pode remover ou reduzir para QoS 0
	go func() {
		// Espera em background para logar erro, se houver (improvável para QoS 0)
		_ = token.WaitTimeout(5 * time.Second)
		if err := token.Error(); err != nil {
			log.Printf("Error during MQTT publish to %s: %v", topic, err)
		}
	}()
	// log.Printf("Published message to %s", topic) // Log pode ser muito verboso
	return nil // Retorna imediatamente para QoS 0
}
