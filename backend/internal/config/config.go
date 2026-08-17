package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort    string
	MQTTBroker    string
	MQTTClientID  string
	MQTTUsername  string
	MQTTPassword  string
	DockStorePath string
}

func LoadConfig() *Config {
	// 加载 .env 文件（不存在时不报错）
	if err := godotenv.Load(); err != nil {
		log.Println("[Config] 未找到 .env 文件，使用系统环境变量")
	}

	return &Config{
		ServerPort:    getEnv("SERVER_PORT", ":8081"),
		MQTTBroker:    getEnv("MQTT_BROKER", "tcp://localhost:1883"),
		MQTTClientID:  getEnv("MQTT_CLIENT_ID", "drone-sim-server"),
		MQTTUsername:  getEnv("MQTT_USERNAME", ""),
		MQTTPassword:  getEnv("MQTT_PASSWORD", ""),
		DockStorePath: getEnv("DOCK_STORE_PATH", "docks.json"),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
