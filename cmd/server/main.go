package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"time"
	"yantai/frontend"
	"yantai/internal/workspace"
)

func loopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}
func main() {
	root := flag.String("data", "data", "小说数据目录")
	address := flag.String("listen", "127.0.0.1:48120", "监听地址")
	remote := flag.Bool("server", false, "仅提供 TLS 加密客户端同步，需设置 YANTAI_SERVER_TOKEN（至少16字符）")
	cert := flag.String("tls-cert", "", "TLS证书文件；未提供时自动生成私有证书")
	key := flag.String("tls-key", "", "TLS私钥文件")
	web := flag.String("web-listen", "", "可选本机网页地址，例如127.0.0.1:48121；不允许公网监听")
	flag.Parse()
	if _, _, err := net.SplitHostPort(*address); err != nil {
		log.Fatal("监听地址需包含主机和端口")
	}
	if !*remote && !loopbackAddress(*address) {
		log.Fatal("远程监听请加 -server，并配置 YANTAI_SERVER_TOKEN")
	}
	if *web != "" && (!*remote || !loopbackAddress(*web) || *web == *address) {
		log.Fatal("web-listen 仅在服务端模式可用，必须为独立的本机回环地址")
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
	server := &workspace.Server{Store: store, Assets: frontend.Assets(), Remote: *remote, AuthKey: secret}
	handler := server.Handler()
	scheme := "http"
	if *remote {
		handler = server.SyncHandler()
		if *cert == "" {
			*cert, *key, err = ensureSyncCertificate(*root)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	if *cert != "" {
		scheme = "https"
		fingerprint, err := certificateFingerprint(*cert, *key)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("TLS证书 SHA-256 指纹（客户端核对后填写）：%s", fingerprint)
	}
	if *web != "" {
		webServer := &http.Server{Addr: *web, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
		go func() { log.Printf("本机网页工作台：http://%s", *web); log.Fatal(webServer.ListenAndServe()) }()
	}
	log.Printf("Yantai已启动：%s://%s", scheme, *address)
	httpServer := &http.Server{Addr: *address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	if *cert != "" {
		log.Fatal(httpServer.ListenAndServeTLS(*cert, *key))
	} else {
		log.Fatal(httpServer.ListenAndServe())
	}
}
