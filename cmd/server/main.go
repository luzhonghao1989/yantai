package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"time"
	"yantai/frontend"
	"yantai/internal/workspace"
)

func main() {
	root := flag.String("data", "data", "小说数据目录")
	address := flag.String("listen", "127.0.0.1:48120", "监听地址")
	remote := flag.Bool("server", false, "允许远程浏览器和客户端同步，需设置 YANTAI_SERVER_TOKEN（至少16字符）")
	cert := flag.String("tls-cert", "", "HTTPS证书文件")
	key := flag.String("tls-key", "", "HTTPS私钥文件")
	secureCookie := flag.Bool("secure-cookie", false, "HTTPS反向代理后启用安全会话Cookie")
	flag.Parse()
	host, _, err := net.SplitHostPort(*address)
	if err != nil {
		log.Fatal("监听地址需包含主机和端口")
	}
	if !*remote && host != "127.0.0.1" && host != "localhost" && host != "::1" {
		log.Fatal("远程监听请加 -server，并配置 YANTAI_SERVER_TOKEN")
	}
	secret := os.Getenv("YANTAI_SERVER_TOKEN")
	if *remote && len(secret) < 16 {
		log.Fatal("YANTAI_SERVER_TOKEN 至少需要16字符")
	}
	if (*cert == "") != (*key == "") {
		log.Fatal("tls-cert 与 tls-key 需要一起提供")
	}
	store, err := workspace.NewStore(*root)
	if err != nil {
		log.Fatal(err)
	}
	server := &workspace.Server{Store: store, Assets: frontend.Assets(), Remote: *remote, AuthKey: secret, SecureCookie: *secureCookie}
	scheme := "http"
	if *cert != "" {
		scheme = "https"
	}
	log.Printf("Yantai已启动：%s://%s", scheme, *address)
	httpServer := &http.Server{Addr: *address, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second}
	if *cert != "" {
		log.Fatal(httpServer.ListenAndServeTLS(*cert, *key))
	} else {
		log.Fatal(httpServer.ListenAndServe())
	}
}
