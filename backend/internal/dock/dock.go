package dock

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"sync"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"drone-sim-backend/internal/mqtt"
	"drone-sim-backend/internal/trajectory"
)

// Config 一台模拟机巢的配置
type Config struct {
	SN          string `json:"sn"`          // 设备序列号，同时作为 MQTT clientId
	DroneSN     string `json:"droneSn"`     // 机巢内无人机的序列号（自动生成）
	Broker      string `json:"broker"`      // MQTT broker 地址，如 tcp://host:port
	Username    string `json:"username"`    // MQTT 认证用户名
	Password    string `json:"password"`    // MQTT 认证密码
	OrgID       string `json:"orgId"`       // 组织 ID
	BindingCode string `json:"bindingCode"` // 绑定码
}

// Info 返回给前端的机巢信息（不含密码）
type Info struct {
	SN          string `json:"sn"`
	DroneSN     string `json:"droneSn"`
	Broker      string `json:"broker"`
	Username    string `json:"username"`
	OrgID       string `json:"orgId"`
	BindingCode string `json:"bindingCode"`
	Status      string `json:"status"` // online | offline
	Error       string `json:"error"`  // 最近一次连接错误（离线时非空）
}

// ServiceHandler 处理 thing/product/{sn}/services 指令。
// reply 用于向该机巢自己的 broker 回复 services_reply。
type ServiceHandler func(sn string, payload []byte, reply func(topic string, payload []byte))

// DRCHandler 处理 thing/product/{sn}/drc/down 指令。
type DRCHandler func(sn string, payload []byte)

type Dock struct {
	cfg     Config
	pub     *mqtt.Publisher
	mu      sync.RWMutex
	lastErr string
}

// Manager 管理所有模拟机巢及其独立的 MQTT 连接
type Manager struct {
	mu        sync.RWMutex
	docks     map[string]*Dock
	storePath string
	onService ServiceHandler
	onDRC     DRCHandler
}

func NewManager(storePath string, onService ServiceHandler, onDRC DRCHandler) *Manager {
	return &Manager{
		docks:     make(map[string]*Dock),
		storePath: storePath,
		onService: onService,
		onDRC:     onDRC,
	}
}

// Start 从持久化文件恢复机巢并连接
func (m *Manager) Start() error {
	configs, err := loadConfigs(m.storePath)
	if err != nil {
		return err
	}
	for _, cfg := range configs {
		m.mu.Lock()
		if _, ok := m.docks[cfg.SN]; ok {
			m.mu.Unlock()
			continue
		}
		m.docks[cfg.SN] = newDock(cfg)
		m.mu.Unlock()
		m.docks[cfg.SN].connect(m.onService, m.onDRC)
	}
	if len(configs) > 0 {
		log.Printf("[Dock] 已从 %s 恢复 %d 台机巢", m.storePath, len(configs))
	}
	return nil
}

// List 返回所有机巢信息
func (m *Manager) List() []Info {
	m.mu.RLock()
	defer m.mu.RUnlock()
	infos := make([]Info, 0, len(m.docks))
	for _, d := range m.docks {
		infos = append(infos, Info{
			SN:          d.cfg.SN,
			DroneSN:     d.cfg.DroneSN,
			Broker:      d.cfg.Broker,
			Username:    d.cfg.Username,
			OrgID:       d.cfg.OrgID,
			BindingCode: d.cfg.BindingCode,
			Status:      d.status(),
			Error:       d.lastError(),
		})
	}
	return infos
}

// Register 注册一台新机巢：持久化并连接其 broker
func (m *Manager) Register(cfg Config) error {
	if cfg.DroneSN == "" {
		cfg.DroneSN = generateDroneSN()
	}

	m.mu.Lock()
	if _, ok := m.docks[cfg.SN]; ok {
		m.mu.Unlock()
		return fmt.Errorf("机巢 %s 已存在", cfg.SN)
	}
	d := newDock(cfg)
	m.docks[cfg.SN] = d
	if err := m.saveLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	d.connect(m.onService, m.onDRC)
	return nil
}

// Update 更新机巢配置（SN 不变）并重连
func (m *Manager) Update(sn string, cfg Config) error {
	cfg.SN = sn // SN 作为主键不可变

	m.mu.Lock()
	d, ok := m.docks[sn]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("机巢 %s 不存在", sn)
	}
	d.pub.Disconnect()
	if cfg.Password == "" {
		cfg.Password = d.cfg.Password // 密码留空则保持不变
	}
	if cfg.DroneSN == "" {
		cfg.DroneSN = d.cfg.DroneSN // 无人机 SN 留空则保持不变
	}
	d.cfg = cfg
	d.pub = mqtt.NewPublisher(cfg.Broker, cfg.SN, cfg.Username, cfg.Password)
	if err := m.saveLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	d.connect(m.onService, m.onDRC)
	return nil
}

// Remove 移除机巢并断开连接
func (m *Manager) Remove(sn string) error {
	m.mu.Lock()
	d, ok := m.docks[sn]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("机巢 %s 不存在", sn)
	}
	d.pub.Disconnect()
	delete(m.docks, sn)
	if err := m.saveLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()
	return nil
}

// Reconnect 手动重连指定机巢
func (m *Manager) Reconnect(sn string) error {
	m.mu.RLock()
	d, ok := m.docks[sn]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("机巢 %s 不存在", sn)
	}
	d.pub.Disconnect()
	d.connect(m.onService, m.onDRC)
	return nil
}

// PublishTelemetry 向指定机巢的 broker 发布遥测
func (m *Manager) PublishTelemetry(sn string, tel trajectory.RemoteIDTelemetry) {
	m.mu.RLock()
	d, ok := m.docks[sn]
	m.mu.RUnlock()
	if !ok {
		return
	}
	d.pub.PublishTelemetry(tel)
}

// PublishModeChange 向指定机巢的 broker 发布状态变更
func (m *Manager) PublishModeChange(sn, status string) {
	m.mu.RLock()
	d, ok := m.docks[sn]
	m.mu.RUnlock()
	if !ok {
		return
	}
	d.pub.PublishModeChange(sn, status)
}

// PublishDeviceOnline 向指定机巢的 broker 发布上线消息
func (m *Manager) PublishDeviceOnline(sn string) {
	m.mu.RLock()
	d, ok := m.docks[sn]
	m.mu.RUnlock()
	if !ok {
		return
	}
	d.pub.PublishDeviceOnline(sn)
}

func newDock(cfg Config) *Dock {
	return &Dock{
		cfg: cfg,
		pub: mqtt.NewPublisher(cfg.Broker, cfg.SN, cfg.Username, cfg.Password),
	}
}

func (d *Dock) status() string {
	if d.pub.IsConnected() {
		return "online"
	}
	return "offline"
}

func (d *Dock) setError(err string) {
	d.mu.Lock()
	d.lastErr = err
	d.mu.Unlock()
}

func (d *Dock) lastError() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastErr
}

// connect 连接机巢的 broker，订阅 services/drc 指令，并发布上线消息
func (d *Dock) connect(onService ServiceHandler, onDRC DRCHandler) {
	sn := d.cfg.SN
	if err := d.pub.Connect(); err != nil {
		d.setError(err.Error())
		log.Printf("[Dock] %s 连接失败: %v", sn, err)
		return
	}

	client := d.pub.Client()

	if token := client.Subscribe("thing/product/"+sn+"/services", 0, func(c pahomqtt.Client, msg pahomqtt.Message) {
		reply := func(topic string, payload []byte) {
			c.Publish(topic, 0, false, payload)
		}
		onService(sn, msg.Payload(), reply)
	}); token.Wait() && token.Error() != nil {
		log.Printf("[Dock] %s 订阅 services 失败: %v", sn, token.Error())
	}

	if token := client.Subscribe("thing/product/"+sn+"/drc/down", 0, func(c pahomqtt.Client, msg pahomqtt.Message) {
		onDRC(sn, msg.Payload())
	}); token.Wait() && token.Error() != nil {
		log.Printf("[Dock] %s 订阅 drc/down 失败: %v", sn, token.Error())
	}

	d.setError("")
	d.pub.PublishDeviceOnline(sn)
	log.Printf("[Dock] %s 已上线（%s）", sn, d.cfg.Broker)
}

// saveLocked 将机巢配置写入 JSON 文件（调用方需持有 m.mu）
func (m *Manager) saveLocked() error {
	configs := make([]Config, 0, len(m.docks))
	for _, d := range m.docks {
		configs = append(configs, d.cfg)
	}
	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.storePath, data, 0644)
}

func loadConfigs(path string) ([]Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var configs []Config
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, err
	}
	return configs, nil
}

// generateDroneSN 生成无人机序列号（16 位大写字母 + 数字）
func generateDroneSN() string {
	const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const n = 16
	b := make([]byte, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// 加密随机源失败时退回确定性字符（极端情况，几乎不会发生）
			b[i] = charset[i%len(charset)]
			continue
		}
		b[i] = charset[idx.Int64()]
	}
	return string(b)
}
