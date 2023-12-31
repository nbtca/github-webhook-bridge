package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var (
	clients   = make(map[*websocket.Conn]bool) //已经链接的ws客户端
	clientsMu sync.Mutex                       //客户端锁
	upgrader  = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
		Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
			log.Println("WebSocket error:", status, reason)
		},
	}
)

// 从客户端收到消息
func handleWsMessage(message []byte) {
}

// 处理ws路径的请求
func handleWs(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		log.Println(err)
		return
	}
	go func() {
		clientsMu.Lock()
		clients[conn] = true
		clientsMu.Unlock()
		defer conn.Close()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				break
			}
			handleWsMessage(message)
		}
	}()
}

type GithubWebhookPost struct {
	Headers map[string][]string `json:"headers"`
	Body    interface{}         `json:"body"`
}

// 处理webhook请求(POST)
func handleWebhook(c *gin.Context) {
	var body interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Println(err)
		return
	}
	headers := make(map[string][]string)
	for k, v := range c.Request.Header {
		headers[k] = v
	}
	go func() {
		fulldata := GithubWebhookPost{
			Headers: headers,
			Body:    body,
		}
		jsonData, err := json.Marshal(fulldata)
		if err != nil {
			log.Println(err)
			return
		}
		fmt.Println(string(jsonData))
		clientsMu.Lock()
		defer clientsMu.Unlock()
		for client := range clients {
			err := client.WriteMessage(websocket.TextMessage, jsonData)
			if err != nil {
				log.Println("Failed to send WebSocket message:", err)
				client.Close()
				delete(clients, client)
			}
		}
	}()
}

type Config struct {
	Bind     string `json:"bind"`      //绑定地址
	UseCert  bool   `json:"use_cert"`  //是否使用证书
	CertFile string `json:"cert_file"` //证书文件
	KeyFile  string `json:"key_file"`  //证书密钥文件
}

var cfg Config

func loadConfig() {
	cfgPath := "config.json"
	if len(os.Args) < 2 {
		log.Println("No config file specified, using default config.json")
	} else {
		cfgPath = os.Args[1]
		log.Println("Using config file:", cfgPath)
	}
	cfgbuf, err := os.ReadFile(cfgPath) //读取配置文件
	if err != nil {
		log.Println("Read config file failed:", err)
		//write default config
		cfgbuf, err = json.MarshalIndent(Config{
			Bind:     ":8080",
			UseCert:  false,
			CertFile: "fullchain.cer",
			KeyFile:  "private.key",
		}, "", "  ")
		os.WriteFile(cfgPath, cfgbuf, 0644)
		if err != nil {
			log.Println("Write default config failed:", err)
			return
		}
	}
	err = json.Unmarshal(cfgbuf, &cfg) //解析配置文件 反序列化json到结构体
	if err != nil {
		log.Println("Unmarshal config failed:", err)
		return
	}
}
func main() {
	loadConfig()
	r := gin.Default()
	r.Use(cors.Default())             //跨域
	r.GET("/", func(c *gin.Context) { //测试
		c.String(http.StatusOK, "test passed")
	})
	r.POST("/webhook", handleWebhook) //webhook服务
	r.GET("/ws", handleWs)            //ws服务
	if cfg.UseCert {
		r.RunTLS(cfg.Bind, cfg.CertFile, cfg.KeyFile) //启动服务
	} else {
		r.Run(cfg.Bind) //启动服务
	}
}
