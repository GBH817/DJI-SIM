package mqtt

import (
	"log"

	mqttserver "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
)

// EmbeddedBroker 内嵌 MQTT Broker
type EmbeddedBroker struct {
	server  *mqttserver.Server
	tcpAddr string
	wsAddr  string
}

// NewEmbeddedBroker 创建内嵌 MQTT Broker
// tcpAddr: TCP 监听地址，供后端 Publisher 和其他 Go/Python/Java 项目连接，如 ":1883"
// wsAddr: WebSocket 监听地址，供浏览器前端（mqtt.js）连接，如 ":1884"
func NewEmbeddedBroker(tcpAddr, wsAddr string) *EmbeddedBroker {
	server := mqttserver.New(&mqttserver.Options{
		InlineClient: true,
	})

	// 允许所有客户端匿名连接（内网使用无需认证）
	if err := server.AddHook(new(auth.AllowHook), nil); err != nil {
		log.Fatalf("[MQTT Broker] 添加认证钩子失败: %v", err)
	}

	return &EmbeddedBroker{
		server:  server,
		tcpAddr: tcpAddr,
		wsAddr:  wsAddr,
	}
}

// Start 启动 Broker，同时监听 TCP 和 WebSocket
func (b *EmbeddedBroker) Start() error {
	// TCP 监听器：供后端 Publisher 和其他项目通过 tcp:// 连接
	tcp := listeners.NewTCP(listeners.Config{
		ID:      "tcp",
		Address: b.tcpAddr,
	})
	if err := b.server.AddListener(tcp); err != nil {
		return err
	}

	// WebSocket 监听器：供浏览器前端（mqtt.js）通过 ws:// 连接
	ws := listeners.NewWebsocket(listeners.Config{
		ID:      "ws",
		Address: b.wsAddr,
	})
	if err := b.server.AddListener(ws); err != nil {
		return err
	}

	go func() {
		if err := b.server.Serve(); err != nil {
			log.Fatalf("[MQTT Broker] 启动失败: %v", err)
		}
	}()

	log.Printf("[MQTT Broker] 内嵌 Broker 已启动 | tcp://%s | ws://%s", b.tcpAddr, b.wsAddr)
	return nil
}

// Stop 停止 Broker
func (b *EmbeddedBroker) Stop() {
	if b.server != nil {
		b.server.Close()
	}
}
